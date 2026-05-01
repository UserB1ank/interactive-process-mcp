package buffer

import (
	"sync"
	"time"
)

// Buffer is a thread-safe ring buffer for process output.
type Buffer struct {
	maxBytes int
	chunks   []string
	total    int
	mu       sync.Mutex
	newData  *sync.Cond
	closed   bool
	readPos  int
	writePos int
}

// New creates a Buffer with the given max capacity in bytes.
func New(maxBytes int) *Buffer {
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}
	b := &Buffer{maxBytes: maxBytes}
	b.newData = sync.NewCond(&b.mu)
	return b
}

// Write appends data to the buffer.
func (b *Buffer) Write(data string) {
	if data == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.chunks = append(b.chunks, data)
	b.total += len(data)
	b.writePos++
	for b.total > b.maxBytes && len(b.chunks) > 1 {
		oldest := b.chunks[0]
		b.chunks = b.chunks[1:]
		b.total -= len(oldest)
		b.readPos--
		if b.readPos < 0 {
			b.readPos = 0
		}
	}
	b.newData.Broadcast()
}

// ReadNew returns all unread data, waiting up to timeout if empty.
// Returns empty string on timeout or if closed.
func (b *Buffer) ReadNew(timeout time.Duration) string {
	b.mu.Lock()
	if b.readPos < b.writePos {
		return b.drainLocked()
	}
	if b.closed {
		b.mu.Unlock()
		return ""
	}
	if timeout <= 0 {
		b.mu.Unlock()
		return ""
	}
	deadline := time.Now().Add(timeout)
	for b.readPos >= b.writePos && !b.closed {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			b.mu.Unlock()
			return ""
		}
		b.newData.Wait()
	}
	if b.closed && b.readPos >= b.writePos {
		b.mu.Unlock()
		return ""
	}
	return b.drainLocked()
}

func (b *Buffer) drainLocked() string {
	var parts []string
	for b.readPos < b.writePos && len(b.chunks) > 0 {
		parts = append(parts, b.chunks[0])
		b.total -= len(b.chunks[0])
		b.chunks = b.chunks[1:]
		b.readPos++
	}
	result := ""
	for _, p := range parts {
		result += p
	}
	b.mu.Unlock()
	return result
}

// Close marks the buffer as closed and wakes waiters.
func (b *Buffer) Close() {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	b.newData.Broadcast()
}
