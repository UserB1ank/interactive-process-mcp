package sshserver

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os/exec"

	"github.com/creack/pty"
	gliderssh "github.com/gliderlabs/ssh"
	"golang.org/x/crypto/ssh"
)

const internalPassword = "interactive-process-internal"

// Server wraps a gliderlabs SSH server for internal use.
type Server struct {
	addr     string
	server   *gliderssh.Server
	listener net.Listener
	started  bool
}

// New creates an internal SSH server listening on addr.
// If addr is empty, it defaults to "127.0.0.1:0" (random port).
func New(addr string) *Server {
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	s := &Server{addr: addr}
	s.server = &gliderssh.Server{
		Addr: addr,
		Handler: func(sess gliderssh.Session) {
			s.handleSession(sess)
		},
		PasswordHandler: func(ctx gliderssh.Context, password string) bool {
			return password == internalPassword
		},
		PtyCallback: func(ctx gliderssh.Context, pty gliderssh.Pty) bool {
			return true
		},
	}
	return s
}

// Addr returns the actual listener address after Start.
func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Start begins listening and serving SSH connections.
func (s *Server) Start() error {
	pemBytes, err := generateHostKeyPEM()
	if err != nil {
		return fmt.Errorf("generate host key: %w", err)
	}
	if err := s.server.SetOption(gliderssh.HostKeyPEM(pemBytes)); err != nil {
		return fmt.Errorf("set host key: %w", err)
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	s.listener = ln
	s.started = true

	go func() {
		if err := s.server.Serve(ln); err != nil {
			// Server closed
		}
	}()
	return nil
}

// Stop shuts down the SSH server.
func (s *Server) Stop() error {
	if !s.started {
		return nil
	}
	return s.server.Close()
}

func (s *Server) handleSession(sess gliderssh.Session) {
	cmdArgs := sess.Command()
	if len(cmdArgs) == 0 {
		io.WriteString(sess, "no command\n")
		sess.Exit(1)
		return
	}

	ptyReq, winCh, isPty := sess.Pty()

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)

	if isPty {
		f, err := pty.StartWithSize(cmd, &pty.Winsize{
			Rows: uint16(ptyReq.Window.Height),
			Cols: uint16(ptyReq.Window.Width),
		})
		if err != nil {
			io.WriteString(sess, err.Error()+"\n")
			sess.Exit(1)
			return
		}
		defer f.Close()

		go func() {
			for win := range winCh {
				pty.Setsize(f, &pty.Winsize{
					Rows: uint16(win.Height),
					Cols: uint16(win.Width),
				})
			}
		}()

		go func() { io.Copy(f, sess) }()
		io.Copy(sess, f)

		cmd.Wait()
	} else {
		cmd.Stdin = sess
		cmd.Stdout = sess
		cmd.Stderr = sess.Stderr()
		cmd.Run()
	}

	sess.Exit(cmd.ProcessState.ExitCode())
}

func generateHostKeyPEM() ([]byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	b := x509.MarshalPKCS1PrivateKey(key)
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: b,
	}
	return pem.EncodeToMemory(block), nil
}

// InternalPassword returns the hardcoded password used for client auth.
func InternalPassword() string {
	return internalPassword
}

// ClientConfig returns a pre-configured ssh.ClientConfig for connecting to the internal server.
func ClientConfig() *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User: "internal",
		Auth: []ssh.AuthMethod{
			ssh.Password(internalPassword),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
}
