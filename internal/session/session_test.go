package session

import (
	"strings"
	"testing"
	"time"

	"github.com/mac01/interactive-process-mcp/internal/sshserver"
	"github.com/mac01/interactive-process-mcp/pkg/api"
)

func startTestServer(t *testing.T) (*sshserver.Server, string) {
	t.Helper()
	srv := sshserver.New("127.0.0.1:0")
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Stop() })
	return srv, srv.Addr()
}

func TestSession_CreateAndInfo(t *testing.T) {
	_, addr := startTestServer(t)

	s, err := New(addr, "bash", nil, "pty", "test-session", 24, 80, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Terminate(true, 0)

	info := s.Info()
	if info.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if info.Name != "test-session" {
		t.Fatalf("expected name 'test-session', got %q", info.Name)
	}
	if info.Status != api.SessionRunning {
		t.Fatalf("expected status 'running', got %q", info.Status)
	}
	if info.Mode != "pty" {
		t.Fatalf("expected mode 'pty', got %q", info.Mode)
	}
}

func TestSession_SendInputReadOutput(t *testing.T) {
	_, addr := startTestServer(t)

	s, err := New(addr, "bash", nil, "pty", "", 24, 80, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Terminate(true, 0)

	time.Sleep(200 * time.Millisecond)

	if err := s.SendInput("echo session_test", true); err != nil {
		t.Fatal(err)
	}

	// Loop-read until we see the expected output
	var output string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		chunk := s.ReadOutput(500*time.Millisecond, true, 0)
		output += chunk
		if strings.Contains(output, "session_test") {
			break
		}
	}
	if !strings.Contains(output, "session_test") {
		t.Fatalf("expected output containing 'session_test', got %q", output)
	}
}

func TestSession_Terminate(t *testing.T) {
	_, addr := startTestServer(t)

	s, err := New(addr, "sleep", []string{"60"}, "pipe", "", 24, 80, nil)
	if err != nil {
		t.Fatal(err)
	}

	info := s.Info()
	if info.Status != api.SessionRunning {
		t.Fatalf("expected 'running', got %q", info.Status)
	}

	s.Terminate(false, 2*time.Second)

	// Wait for exit goroutine to update status
	time.Sleep(200 * time.Millisecond)

	info = s.Info()
	if info.Status != api.SessionExited {
		t.Fatalf("expected 'exited', got %q", info.Status)
	}
	if info.ExitCode == nil {
		t.Fatal("expected non-nil exit code")
	}
}

func TestSession_ForceTerminate(t *testing.T) {
	_, addr := startTestServer(t)

	s, err := New(addr, "sleep", []string{"60"}, "pipe", "", 24, 80, nil)
	if err != nil {
		t.Fatal(err)
	}

	s.Terminate(true, 0)

	time.Sleep(200 * time.Millisecond)
	info := s.Info()
	if info.Status != api.SessionExited {
		t.Fatalf("expected 'exited' after force terminate, got %q", info.Status)
	}
}

func TestSession_ResizePty(t *testing.T) {
	_, addr := startTestServer(t)

	s, err := New(addr, "bash", nil, "pty", "", 24, 80, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Terminate(true, 0)

	if err := s.ResizePty(50, 120); err != nil {
		t.Fatalf("ResizePty failed: %v", err)
	}

	info := s.Info()
	if info.Rows != 50 || info.Cols != 120 {
		t.Fatalf("expected 50x120, got %dx%d", info.Rows, info.Cols)
	}
}

func TestSession_ResizePtyPipeMode(t *testing.T) {
	_, addr := startTestServer(t)

	s, err := New(addr, "cat", nil, "pipe", "", 24, 80, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Terminate(true, 0)

	err = s.ResizePty(50, 120)
	if err == nil {
		t.Fatal("expected error when resizing PTY in pipe mode")
	}
}

func TestSession_SendInputAfterExit(t *testing.T) {
	_, addr := startTestServer(t)

	// Use PTY mode to avoid pipe mode stdin deadlock
	s, err := New(addr, "bash", nil, "pty", "", 24, 80, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Tell bash to exit
	s.SendInput("exit", true)

	// Wait for the process to exit
	time.Sleep(1 * time.Second)

	err = s.SendInput("should fail", true)
	if err == nil {
		t.Fatal("expected error sending input to exited process")
	}
}

func TestSession_NaturalExit(t *testing.T) {
	_, addr := startTestServer(t)

	// Use PTY mode to avoid pipe mode stdin deadlock
	s, err := New(addr, "bash", []string{"-c", "echo hello"}, "pty", "", 24, 80, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for natural exit
	time.Sleep(2 * time.Second)

	info := s.Info()
	if info.Status != api.SessionExited {
		t.Fatalf("expected 'exited', got %q", info.Status)
	}
	if info.ExitCode == nil || *info.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %v", info.ExitCode)
	}
}

func TestManager_CreateAndGet(t *testing.T) {
	_, addr := startTestServer(t)

	mgr := NewManager(addr, nil, nil)

	s, err := mgr.Create("echo", []string{"hi"}, "pipe", "test", 24, 80)
	if err != nil {
		t.Fatal(err)
	}

	got := mgr.Get(s.ID)
	if got == nil {
		t.Fatal("expected to find session")
	}
	if got.ID != s.ID {
		t.Fatalf("expected ID %q, got %q", s.ID, got.ID)
	}
}

func TestManager_ListAll(t *testing.T) {
	_, addr := startTestServer(t)

	mgr := NewManager(addr, nil, nil)

	mgr.Create("echo", []string{"a"}, "pipe", "s1", 24, 80)
	mgr.Create("echo", []string{"b"}, "pipe", "s2", 24, 80)

	all := mgr.ListAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(all))
	}
}

func TestManager_CleanupAll(t *testing.T) {
	_, addr := startTestServer(t)

	mgr := NewManager(addr, nil, nil)

	mgr.Create("sleep", []string{"60"}, "pipe", "s1", 24, 80)
	mgr.Create("sleep", []string{"60"}, "pipe", "s2", 24, 80)

	mgr.CleanupAll(true)

	time.Sleep(500 * time.Millisecond)

	for _, s := range mgr.ListAll() {
		if s.Status != api.SessionExited {
			t.Fatalf("expected all sessions exited, got %q for %s", s.Status, s.ID)
		}
	}
}
