package buffer

import (
	"errors"
	"io"
	"sync"
	"time"

	"github.com/smallnest/ringbuffer"
)

const DefaultMaxBytes = 1024 * 1024

var (
	ErrClosed = errors.New("buffer: closed")
	ErrReader = errors.New("buffer: invalid reader ID")
)

// Buffer is a thread-safe multi-reader ring buffer for process output.
// Each reader gets its own independent view of the data via a ringbuffer
// that supports overwrite semantics.
type Buffer struct {
	mu      sync.Mutex
	size    int
	readers map[int]*ringbuffer.RingBuffer
	nextID  int
	closed  bool
	cond    *sync.Cond
}

// New creates a Buffer with the given max capacity in bytes.
func New(maxBytes int) *Buffer {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	b := &Buffer{
		size:    maxBytes,
		readers: make(map[int]*ringbuffer.RingBuffer),
	}
	b.cond = sync.NewCond(&b.mu)
	return b
}

// NewReader registers a new independent reader and returns its ID.
func (b *Buffer) NewReader() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.nextID
	b.nextID++
	rb := ringbuffer.New(b.size)
	rb.SetOverwrite(true)
	b.readers[id] = rb
	return id
}

// Unregister removes a reader by ID.
func (b *Buffer) Unregister(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.readers, id)
}

// Write appends data to the buffer, broadcasting to all waiting readers.
func (b *Buffer) Write(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrClosed
	}
	for _, rb := range b.readers {
		rb.Write(data)
	}
	b.cond.Broadcast()
	return nil
}

// Read reads all available data for the given reader.
// If no data is available, it waits up to timeout.
// Returns (nil, ErrReader) for invalid reader IDs.
// Returns (nil, io.EOF) if the buffer is closed.
func (b *Buffer) Read(readerID int, timeout time.Duration) ([]byte, error) {
	b.mu.Lock()
	rb, ok := b.readers[readerID]
	if !ok {
		b.mu.Unlock()
		return nil, ErrReader
	}

	if length := rb.Length(); length > 0 {
		buf := make([]byte, length)
		n, _ := rb.Read(buf)
		b.mu.Unlock()
		return buf[:n], nil
	}

	if b.closed {
		b.mu.Unlock()
		return nil, io.EOF
	}

	if timeout <= 0 {
		b.mu.Unlock()
		return nil, nil
	}

	deadline := time.Now().Add(timeout)
	timer := time.AfterFunc(timeout, b.cond.Broadcast)
	defer timer.Stop()

	for rb.Length() == 0 && !b.closed {
		if time.Until(deadline) <= 0 {
			b.mu.Unlock()
			return nil, nil
		}
		b.cond.Wait()
	}

	if length := rb.Length(); length > 0 {
		buf := make([]byte, length)
		n, _ := rb.Read(buf)
		b.mu.Unlock()
		return buf[:n], nil
	}

	b.mu.Unlock()
	if b.closed {
		return nil, io.EOF
	}
	return nil, nil
}

// HasMore returns whether the given reader has unread data.
func (b *Buffer) HasMore(readerID int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	rb, ok := b.readers[readerID]
	if !ok {
		return false
	}
	return rb.Length() > 0
}

// Close marks the buffer as closed and wakes all waiting readers.
func (b *Buffer) Close() {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	b.cond.Broadcast()
}
