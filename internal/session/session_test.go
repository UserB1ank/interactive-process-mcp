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

func testConfig(command string, args []string, mode string, name string) Config {
	return Config{
		Command: command,
		Args:    args,
		Mode:    mode,
		Name:    name,
		Rows:    24,
		Cols:    80,
	}
}

func TestSession_CreateAndInfo(t *testing.T) {
	_, addr := startTestServer(t)

	s, err := New(addr, testConfig("bash", nil, "pty", "test-session"), nil)
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

	s, err := New(addr, testConfig("bash", nil, "pty", ""), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Terminate(true, 0)

	time.Sleep(200 * time.Millisecond)

	if err := s.SendInput("echo session_test", true); err != nil {
		t.Fatal(err)
	}

	var output string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		chunk, _ := s.ReadOutput(500*time.Millisecond, true, 0)
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

	s, err := New(addr, testConfig("sleep", []string{"60"}, "pipe", ""), nil)
	if err != nil {
		t.Fatal(err)
	}

	info := s.Info()
	if info.Status != api.SessionRunning {
		t.Fatalf("expected 'running', got %q", info.Status)
	}

	s.Terminate(false, 2*time.Second)

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

	s, err := New(addr, testConfig("sleep", []string{"60"}, "pipe", ""), nil)
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

	s, err := New(addr, testConfig("bash", nil, "pty", ""), nil)
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

	s, err := New(addr, testConfig("cat", nil, "pipe", ""), nil)
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

	s, err := New(addr, testConfig("bash", nil, "pty", ""), nil)
	if err != nil {
		t.Fatal(err)
	}

	s.SendInput("exit", true)

	time.Sleep(1 * time.Second)

	err = s.SendInput("should fail", true)
	if err == nil {
		t.Fatal("expected error sending input to exited process")
	}
}

func TestSession_NaturalExit(t *testing.T) {
	_, addr := startTestServer(t)

	s, err := New(addr, Config{Command: "bash", Args: []string{"-c", "echo hello"}, Mode: "pty", Rows: 24, Cols: 80}, nil)
	if err != nil {
		t.Fatal(err)
	}

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

	s, err := mgr.Create(Config{Command: "echo", Args: []string{"hi"}, Mode: "pipe", Name: "test", Rows: 24, Cols: 80})
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

	mgr.Create(Config{Command: "echo", Args: []string{"a"}, Mode: "pipe", Name: "s1", Rows: 24, Cols: 80})
	mgr.Create(Config{Command: "echo", Args: []string{"b"}, Mode: "pipe", Name: "s2", Rows: 24, Cols: 80})

	all := mgr.ListAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(all))
	}
}

func TestManager_CleanupAll(t *testing.T) {
	_, addr := startTestServer(t)

	mgr := NewManager(addr, nil, nil)

	mgr.Create(Config{Command: "sleep", Args: []string{"60"}, Mode: "pipe", Name: "s1", Rows: 24, Cols: 80})
	mgr.Create(Config{Command: "sleep", Args: []string{"60"}, Mode: "pipe", Name: "s2", Rows: 24, Cols: 80})

	mgr.CleanupAll(true)

	time.Sleep(500 * time.Millisecond)

	for _, s := range mgr.ListAll() {
		if s.Status != api.SessionExited {
			t.Fatalf("expected all sessions exited, got %q for %s", s.Status, s.ID)
		}
	}
}

func TestManager_Delete(t *testing.T) {
	_, addr := startTestServer(t)

	mgr := NewManager(addr, nil, nil)

	s, err := mgr.Create(Config{Command: "echo", Args: []string{"hi"}, Mode: "pipe", Name: "del-me", Rows: 24, Cols: 80})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond)

	if mgr.Get(s.ID) == nil {
		t.Fatal("expected session to exist")
	}

	mgr.Delete(s.ID)

	if mgr.Get(s.ID) != nil {
		t.Fatal("expected session to be deleted")
	}

	all := mgr.ListAll()
	if len(all) != 0 {
		t.Fatalf("expected 0 sessions after delete, got %d", len(all))
	}
}
