package session

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mac01/interactive-process-mcp/internal/sshserver"
	"github.com/mac01/interactive-process-mcp/pkg/api"
)

func startRaceTestServer(t *testing.T) string {
	t.Helper()
	srv := sshserver.New("127.0.0.1:0")
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Stop() })
	return srv.Addr()
}

func TestRace_ConcurrentSendInput(t *testing.T) {
	addr := startRaceTestServer(t)

	s, err := New(addr, "bash", nil, "pty", "", 24, 80, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Terminate(true, 0)

	time.Sleep(300 * time.Millisecond)

	var wg sync.WaitGroup
	errCount := int64(0)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			err := s.SendInput("echo concurrent"+strings.Repeat(" ", id%3), true)
			if err != nil {
				atomic.AddInt64(&errCount, 1)
			}
		}(i)
	}
	wg.Wait()

	// Some writes may race with exit — that's fine, we just verify no panic
	t.Logf("concurrent sends completed, %d errors (expected ~0)", atomic.LoadInt64(&errCount))
}

func TestRace_SendInputDuringTerminate(t *testing.T) {
	addr := startRaceTestServer(t)

	s, err := New(addr, "bash", nil, "pty", "", 24, 80, nil)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	var wg sync.WaitGroup
	errCount := int64(0)

	// Concurrent terminate + send input
	wg.Add(2)
	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
		s.Terminate(true, 0)
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			err := s.SendInput("echo race_test", true)
			if err != nil {
				atomic.AddInt64(&errCount, 1)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	wg.Wait()

	time.Sleep(200 * time.Millisecond)
	info := s.Info()
	if info.Status != api.SessionExited {
		t.Fatalf("expected 'exited', got %q", info.Status)
	}
	t.Logf("send-during-terminate completed, %d send errors (expected some)", atomic.LoadInt64(&errCount))
}

func TestRace_ConcurrentTerminate(t *testing.T) {
	addr := startRaceTestServer(t)

	s, err := New(addr, "sleep", []string{"60"}, "pipe", "", 24, 80, nil)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Terminate(false, 1*time.Second)
		}()
	}
	wg.Wait()

	time.Sleep(300 * time.Millisecond)
	info := s.Info()
	if info.Status != api.SessionExited {
		t.Fatalf("expected 'exited' after concurrent terminate, got %q", info.Status)
	}
}

func TestRace_ConcurrentReadWrite(t *testing.T) {
	addr := startRaceTestServer(t)

	s, err := New(addr, "bash", nil, "pty", "", 24, 80, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Terminate(true, 0)

	time.Sleep(300 * time.Millisecond)

	var wg sync.WaitGroup

	// Concurrent writers
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				s.SendInput("echo rw"+strings.Repeat(" ", id), true)
				time.Sleep(20 * time.Millisecond)
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				s.ReadOutput(200*time.Millisecond, true, 0)
			}
		}()
	}

	wg.Wait()
}

func TestRace_ConcurrentInfoAndTerminate(t *testing.T) {
	addr := startRaceTestServer(t)

	s, err := New(addr, "sleep", []string{"60"}, "pipe", "", 24, 80, nil)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup

	// Readers calling Info() concurrently with Terminate
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				info := s.Info()
				_ = info.Status
				time.Sleep(5 * time.Millisecond)
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		s.Terminate(true, 0)
	}()

	wg.Wait()
}
