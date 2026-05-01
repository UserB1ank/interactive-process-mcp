package sshclient

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mac01/interactive-process-mcp/internal/sshserver"
	"golang.org/x/crypto/ssh"
)

// ExecSession wraps an active SSH client session.
type ExecSession struct {
	client   *ssh.Client
	session  *ssh.Session
	Stdin    io.WriteCloser
	Stdout   io.Reader
	Stderr   io.Reader
	done     chan struct{}
	exitCode int
	err      error
}

// Start connects to the internal SSH server and starts a command.
// addr is the SSH server address (e.g. "127.0.0.1:2222").
// If pty is true, a pseudo-terminal is requested with the given dimensions.
func Start(addr string, command string, args []string, pty bool, rows, cols int) (*ExecSession, error) {
	config := sshserver.ClientConfig()
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("ssh dial: %w", err)
	}

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("new session: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		stdin.Close()
		session.Close()
		client.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		stdout.(io.Closer).Close()
		stdin.Close()
		session.Close()
		client.Close()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if pty {
		modes := ssh.TerminalModes{
			ssh.ECHO:          1,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		}
		if err := session.RequestPty("xterm-256color", rows, cols, modes); err != nil {
			stderr.(io.Closer).Close()
			stdout.(io.Closer).Close()
			stdin.Close()
			session.Close()
			client.Close()
			return nil, fmt.Errorf("request pty: %w", err)
		}
	}

	cmdStr := shellQuote(command, args)
	if err := session.Start(cmdStr); err != nil {
		stderr.(io.Closer).Close()
		stdout.(io.Closer).Close()
		stdin.Close()
		session.Close()
		client.Close()
		return nil, fmt.Errorf("start command: %w", err)
	}

	es := &ExecSession{
		client:  client,
		session: session,
		Stdin:   stdin,
		Stdout:  stdout,
		Stderr:  stderr,
		done:    make(chan struct{}),
	}

	go func() {
		es.err = session.Wait()
		if es.err != nil {
			if exitErr, ok := es.err.(*ssh.ExitError); ok {
				es.exitCode = exitErr.ExitStatus()
			}
		}
		close(es.done)
	}()

	return es, nil
}

// Done returns a channel that closes when the remote process exits.
func (es *ExecSession) Done() <-chan struct{} {
	return es.done
}

// ExitCode returns the process exit code after Done is closed.
func (es *ExecSession) ExitCode() int {
	return es.exitCode
}

// ResizePty sends a window-change request for the session.
func (es *ExecSession) ResizePty(rows, cols int) error {
	return es.session.WindowChange(rows, cols)
}

// Signal sends a signal to the remote process.
func (es *ExecSession) Signal(sig ssh.Signal) error {
	return es.session.Signal(sig)
}

// Close forcefully terminates the session and underlying connection.
func (es *ExecSession) Close() error {
	es.session.Close()
	es.client.Close()
	return nil
}

// shellQuote builds a shell-safe command string.
func shellQuote(command string, args []string) string {
	parts := []string{command}
	for _, a := range args {
		if strings.ContainsAny(a, " \t\n\"'\\$|&;<>(){}[]*?#~`") {
			parts = append(parts, strconv.Quote(a))
		} else {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}
