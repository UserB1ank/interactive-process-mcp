package buffer

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBuffer_WriteAndRead(t *testing.T) {
	b := New(1024)
	b.Write("hello")
	b.Write(" world")

	got := b.ReadNew(time.Second)
	if got != "hello world" {
		t.Fatalf("expected 'hello world', got %q", got)
	}

	// Second read with no new data should return empty immediately
	got = b.ReadNew(0)
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestBuffer_ReadNewWaitsForData(t *testing.T) {
	b := New(1024)
	done := make(chan struct{})

	go func() {
		time.Sleep(100 * time.Millisecond)
		b.Write("delayed")
		close(done)
	}()

	got := b.ReadNew(2 * time.Second)
	if got != "delayed" {
		t.Fatalf("expected 'delayed', got %q", got)
	}
	<-done
}

func TestBuffer_ReadNewTimeout(t *testing.T) {
	b := New(1024)
	start := time.Now()
	got := b.ReadNew(200 * time.Millisecond)
	elapsed := time.Since(start)
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if elapsed < 150*time.Millisecond {
		t.Fatalf("returned too fast: %v", elapsed)
	}
}

func TestBuffer_RingEviction(t *testing.T) {
	b := New(32)
	// Write 3 chunks of 16 bytes each — should evict the first one
	b.Write(strings.Repeat("a", 16))
	b.Write(strings.Repeat("b", 16))
	b.Write(strings.Repeat("c", 16))

	got := b.ReadNew(time.Second)
	// First chunk should be evicted; we get b's + c's
	if !strings.Contains(got, strings.Repeat("b", 16)) {
		t.Fatalf("expected 'b' chunk to survive, got %q", got)
	}
	if !strings.Contains(got, strings.Repeat("c", 16)) {
		t.Fatalf("expected 'c' chunk to survive, got %q", got)
	}
}

func TestBuffer_CloseWakesReaders(t *testing.T) {
	b := New(1024)
	done := make(chan struct{})

	go func() {
		got := b.ReadNew(10 * time.Second)
		if got != "" {
			t.Errorf("expected empty on close, got %q", got)
		}
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	b.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not wake the reader")
	}
}

func TestBuffer_WriteAfterClose(t *testing.T) {
	b := New(1024)
	b.Write("before")
	b.Close()
	b.Write("after") // should be silently ignored

	got := b.ReadNew(time.Second)
	if got != "before" {
		t.Fatalf("expected 'before', got %q", got)
	}
}

func TestBuffer_ConcurrentReadWrite(t *testing.T) {
	b := New(1024 * 1024)
	var wg sync.WaitGroup

	// Multiple concurrent writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				b.Write("w")
			}
		}(i)
	}

	// One reader
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
		b.ReadNew(2 * time.Second)
	}()

	wg.Wait()
}

func TestBuffer_SpuriousWakeup(t *testing.T) {
	b := New(1024)

	// Simulate spurious wakeup by broadcasting with no data
	go func() {
		time.Sleep(100 * time.Millisecond)
		b.newData.Broadcast() // spurious
		time.Sleep(100 * time.Millisecond)
		b.Write("real data")
	}()

	got := b.ReadNew(2 * time.Second)
	if got != "real data" {
		t.Fatalf("expected 'real data' after spurious wakeup, got %q", got)
	}
}
