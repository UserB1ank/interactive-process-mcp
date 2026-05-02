package session

import (
	"fmt"
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

// Session wraps an interactive process session managed over SSH.
type Session struct {
	api.Session
	mu            sync.RWMutex
	stdinMu       sync.Mutex
	terminateOnce sync.Once
	execSession   *sshclient.ExecSession
	buf           *buffer.Buffer
	readerID      int
	msgMgr        *message.Manager
	sshAddr       string
}

// New creates and starts a new Session.
func New(sshAddr string, command string, args []string, mode string, name string, rows, cols int, msgMgr *message.Manager) (*Session, error) {
	id := uuid.New().String()[:12]
	if name == "" {
		name = fmt.Sprintf("session-%s", id)
	}

	usePty := mode == "pty"
	execSession, err := sshclient.Start(sshAddr, command, args, usePty, rows, cols)
	if err != nil {
		return nil, err
	}

	buf := buffer.New(1024 * 1024)

	s := &Session{
		Session: api.Session{
			ID:        id,
			Name:      name,
			Command:   command,
			Args:      args,
			Mode:      mode,
			Status:    api.SessionRunning,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
			Rows:      rows,
			Cols:      cols,
		},
		execSession: execSession,
		buf:         buf,
		readerID:    buf.NewReader(),
		msgMgr:      msgMgr,
		sshAddr:     sshAddr,
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
				s.buf.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := s.execSession.Stderr.Read(buf)
			if n > 0 {
				s.buf.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
	}()

	go func() {
		<-s.execSession.Done()
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
	}()
}

// SendInput writes text to the process stdin.
func (s *Session) SendInput(text string, pressEnter bool) error {
	s.mu.RLock()
	if s.Status != api.SessionRunning {
		s.mu.RUnlock()
		return fmt.Errorf("process has %s, cannot send input", s.Status)
	}
	if pressEnter {
		text += "\n"
	}
	s.stdinMu.Lock()
	_, err := s.execSession.Stdin.Write([]byte(text))
	s.stdinMu.Unlock()
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	if s.msgMgr != nil {
		s.msgMgr.Append(s.ID, api.MsgInput, text)
	}
	return nil
}

// ReadOutput reads new output from the buffer.
func (s *Session) ReadOutput(timeout time.Duration, stripAnsi bool, maxLines int) string {
	data, _ := s.buf.Read(s.readerID, timeout)
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
	return output
}

// Terminate gracefully or forcefully stops the process.
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

		s.mu.Lock()
		if s.Status == api.SessionRunning {
			s.Status = api.SessionExited
			code := -1
			s.ExitCode = &code
			s.UpdatedAt = time.Now().UTC()
		}
		s.mu.Unlock()

		if s.msgMgr != nil {
			s.msgMgr.Append(s.ID, api.MsgSystem, "Process terminated")
		}
	})
}

// ResizePty adjusts the terminal dimensions (pty mode only).
func (s *Session) ResizePty(rows, cols int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Status != api.SessionRunning {
		return fmt.Errorf("process not running")
	}
	if s.Mode != "pty" {
		return fmt.Errorf("PTY resize only available in pty mode")
	}
	if err := s.execSession.ResizePty(rows, cols); err != nil {
		return err
	}
	s.Rows = rows
	s.Cols = cols
	return nil
}

// Info returns a copy of the session metadata.
func (s *Session) Info() api.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Session
}

// ReadOutputForReader reads new output for a specific reader ID.
func (s *Session) ReadOutputForReader(readerID int, timeout time.Duration, stripAnsi bool, maxLines int) string {
	data, _ := s.buf.Read(readerID, timeout)
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
	return output
}

// RegisterReader creates a new independent reader and returns its ID.
func (s *Session) RegisterReader() int {
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
