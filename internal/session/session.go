package session

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mac01/interactive-process-mcp/pkg/api"
	"github.com/mac01/interactive-process-mcp/internal/ansi"
	"github.com/mac01/interactive-process-mcp/internal/buffer"
	"github.com/mac01/interactive-process-mcp/internal/message"
	"github.com/mac01/interactive-process-mcp/internal/sshclient"
	"golang.org/x/crypto/ssh"
)

// Lock ordering: mu -> stdinMu. Never acquire in reverse order.

// Config holds parameters for creating a new Session.
type Config struct {
	Command string
	Args    []string
	Mode    api.SessionMode
	Name    string
	Rows    int
	Cols    int
}

// Session wraps an interactive process session managed over SSH.
type Session struct {
	api.Session
	mu            sync.RWMutex
	stdinMu       sync.Mutex
	terminateOnce sync.Once
	exitOnce      sync.Once
	execSession   *sshclient.ExecSession
	buf           *buffer.Buffer
	readerID      int
	msgMgr        *message.Manager
	sshAddr       string
	done          chan struct{}
}

// New creates and starts a new Session.
func New(sshAddr string, cfg Config, msgMgr *message.Manager) (*Session, error) {
	id := uuid.New().String()[:12]
	name := cfg.Name
	if name == "" {
		name = fmt.Sprintf("session-%s", id)
	}

	usePty := cfg.Mode == api.ModePTY
	execSession, err := sshclient.Start(sshAddr, cfg.Command, cfg.Args, usePty, cfg.Rows, cfg.Cols)
	if err != nil {
		return nil, err
	}

	buf := buffer.New(1024 * 1024)
	rid, _ := buf.NewReader()

	s := &Session{
		Session: api.Session{
			ID:        id,
			Name:      name,
			Command:   cfg.Command,
			Args:      cfg.Args,
			Mode:      cfg.Mode,
			Status:    api.SessionRunning,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
			Rows:      cfg.Rows,
			Cols:      cfg.Cols,
		},
		execSession: execSession,
		buf:         buf,
		readerID:    rid,
		msgMgr:      msgMgr,
		sshAddr:     sshAddr,
		done:        make(chan struct{}),
	}

	s.startReaders()

	if msgMgr != nil {
		msgMgr.Append(s.ID, api.MsgSystem, "Process started")
	}

	return s, nil
}

func (s *Session) startReaders() {
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := s.execSession.Stdout.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				s.buf.Write(data)
			}
			if err != nil {
				return
			}
			select {
			case <-s.done:
				return
			default:
			}
		}
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := s.execSession.Stderr.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				s.buf.Write(data)
			}
			if err != nil {
				return
			}
			select {
			case <-s.done:
				return
			default:
			}
		}
	}()

	go func() {
		<-s.execSession.Done()
		close(s.done)
		s.exitOnce.Do(func() {
			s.mu.Lock()
			s.Status = api.SessionExited
			code := s.execSession.ExitCode()
			s.ExitCode = &code
			s.UpdatedAt = time.Now().UTC()
			s.mu.Unlock()
			s.buf.Close()
			if s.msgMgr != nil {
				s.msgMgr.Append(s.ID, api.MsgSystem, fmt.Sprintf("Process exited with code %d", code))
			}
		})
	}()
}

// SendInput writes text to the process stdin.
func (s *Session) SendInput(text string, pressEnter bool) error {
	s.mu.RLock()
	running := s.Status == api.SessionRunning
	s.mu.RUnlock()
	if !running {
		return fmt.Errorf("process has %s, cannot send input", s.Status)
	}
	if pressEnter {
		text += "\n"
	}
	s.stdinMu.Lock()
	_, err := s.execSession.Stdin.Write([]byte(text))
	s.stdinMu.Unlock()
	if err != nil {
		return err
	}
	if s.msgMgr != nil {
		s.msgMgr.Append(s.ID, api.MsgInput, text)
	}
	return nil
}

func (s *Session) readOutput(readerID int, timeout time.Duration, stripAnsi bool, maxLines int) (string, error) {
	data, err := s.buf.Read(readerID, timeout)
	if err != nil && err != io.EOF {
		return "", err
	}
	output := string(data)
	if stripAnsi {
		output = ansi.Strip(output)
	}
	if maxLines > 0 {
		lines := strings.Split(output, "\n")
		if len(lines) > maxLines {
			output = strings.Join(lines[:maxLines], "\n")
		}
	}
	if output != "" && s.msgMgr != nil {
		s.msgMgr.Append(s.ID, api.MsgOutput, output)
	}
	return output, nil
}

// ReadOutput reads new output using the default reader.
func (s *Session) ReadOutput(timeout time.Duration, stripAnsi bool, maxLines int) (string, error) {
	return s.readOutput(s.readerID, timeout, stripAnsi, maxLines)
}

// ReadOutputForReader reads new output for a specific reader ID.
func (s *Session) ReadOutputForReader(readerID int, timeout time.Duration, stripAnsi bool, maxLines int) (string, error) {
	return s.readOutput(readerID, timeout, stripAnsi, maxLines)
}

// Terminate gracefully or forcefully stops the process.
// The exit goroutine is the single authority for final Status/ExitCode.
func (s *Session) Terminate(force bool, gracePeriod time.Duration) {
	s.terminateOnce.Do(func() {
		if !force {
			s.execSession.Signal(ssh.SIGTERM)
			select {
			case <-s.execSession.Done():
				return
			case <-time.After(gracePeriod):
			}
		}

		s.execSession.Close()

		select {
		case <-s.execSession.Done():
		case <-time.After(2 * time.Second):
		}

		s.exitOnce.Do(func() {
			s.mu.Lock()
			s.Status = api.SessionExited
			code := -1
			s.ExitCode = &code
			s.UpdatedAt = time.Now().UTC()
			s.mu.Unlock()
			s.buf.Close()
			if s.msgMgr != nil {
				s.msgMgr.Append(s.ID, api.MsgSystem, "Process terminated (no exit code)")
			}
		})
	})
}

// ResizePty adjusts the terminal dimensions (pty mode only).
func (s *Session) ResizePty(rows, cols int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Status != api.SessionRunning {
		return fmt.Errorf("process not running")
	}
	if s.Mode != api.ModePTY {
		return fmt.Errorf("PTY resize only available in pty mode")
	}
	if err := s.execSession.ResizePty(rows, cols); err != nil {
		return err
	}
	s.Rows = rows
	s.Cols = cols
	return nil
}

// Info returns a deep copy of the session metadata.
func (s *Session) Info() api.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := s.Session
	if cp.ExitCode != nil {
		v := *cp.ExitCode
		cp.ExitCode = &v
	}
	return cp
}

// RegisterReader creates a new independent reader and returns its ID.
func (s *Session) RegisterReader() (int, error) {
	return s.buf.NewReader()
}

// UnregisterReader removes a reader by ID.
func (s *Session) UnregisterReader(id int) {
	s.buf.Unregister(id)
}

// HasMoreOutput returns whether the given reader has unread data.
func (s *Session) HasMoreOutput(readerID int) bool {
	return s.buf.HasMore(readerID)
}
