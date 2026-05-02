# Multi-Reader Ring Buffer Design

## Problem

The current `internal/buffer/buffer.go` is a single-reader slice-based queue with multiple defects:
- Not a true ring buffer (uses `append` + slice head deletion causing memory churn)
- Single `readPos` means only one consumer can read; multiple agents on one session lose data
- `readPos/writePos` + manual slice trimming create conflicting state
- `drainLocked` does string concatenation under lock (fat critical section)
- `drainLocked` hides mutex unlock in a private method
- Write silently drops data on closed buffer
- Eviction logic breaks when a single oversized chunk exceeds `maxBytes`
- Hardcoded magic values, missing error handling

Additional concurrency bugs in `session.go`:
- `ResizePty` modifies `Rows`/`Cols` under `RLock` (data race)
- `SendInput` has no mutex around stdin writes (concurrent writes may interleave)
- `Terminate` has a lock gap allowing double-terminate
- `has_more` hardcoded to `false`

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Reader model | Per-reader sequence cursor | Each agent tracks its own progress independently |
| Eviction policy | Slowest-reader-driven + hard limit | Safe by default, hard limit prevents OOM |
| Ring storage | Pre-allocated fixed array with head/tail pointers | Zero memory relocation, true O(1) write |
| Sequence tracking | Monotonically increasing int64 per chunk | Single dimension of progress, no pointer conflicts |
| Lock scope | Minimal critical section, string building outside lock | Reduces contention |

## Architecture

### Core Structure

```go
const (
    DefaultMaxBytes = 1024 * 1024 // 1MB soft limit
    HardLimitFactor = 2           // hard limit = maxBytes * 2
    DefaultRingCap  = 4096        // pre-allocated ring capacity (number of entries)
)

type entry struct {
    data []byte
    seq  int64 // monotonically increasing sequence number
}

type reader struct {
    seq int64 // sequence number this reader has consumed up to
}

type Buffer struct {
    maxBytes  int
    hardBytes int // hardBytes = maxBytes * HardLimitFactor
    mu        sync.Mutex
    cond      *sync.Cond
    closed    bool

    ring  []entry // pre-allocated fixed array
    cap   int     // len(ring)
    head  int     // next write position (circular index)
    tail  int     // oldest data position (circular index)
    count int     // number of occupied entries
    total int     // total bytes across all entries
    seq   int64   // next sequence number to assign

    readers map[int]*reader
    nextID  int
}
```

### API

```go
// Constructor
func New(maxBytes int) *Buffer

// Writer (SSH process goroutines)
func (b *Buffer) Write(data []byte) error

// Reader lifecycle
func (b *Buffer) NewReader() (id int, startSeq int64)
func (b *Buffer) Unregister(id int)

// Reader consumption
func (b *Buffer) Read(readerID int, timeout time.Duration) ([]byte, error)
// Returns io.EOF when buffer is closed and no more data
// Returns ErrDataLost when a slow reader's data was force-evicted

// Status
func (b *Buffer) HasMore(readerID int) bool
func (b *Buffer) Close()
```

### Write Logic

1. Lock
2. If closed, return error (not silent drop)
3. Store data at `ring[head]`, set `ring[head].seq = b.seq`, increment `seq`
   - Advance `head = (head + 1) % cap`, increment `count`
   - If ring was full (`count == cap` before store), tail also advances (oldest overwritten)
4. `total += len(data)`
5. Evict:
   - **Soft eviction** (`total > maxBytes`): find `minReadSeq = min(all readers' seq)`, evict entries with `seq < minReadSeq` from tail
   - **Hard eviction** (`total > hardBytes`): evict oldest entry from tail regardless of readers
6. `cond.Broadcast()`
7. Unlock

### Read Logic (per reader)

1. Lock
2. Find reader's `seq` from `readers[readerID]`
3. Scan from `tail` forward to find entries with `seq > reader.seq`:
   - If found: collect `[][]byte` references into local slice, update `reader.seq`, unlock, join with `bytes.Buffer` outside lock
   - If not found and not closed: enter `cond.Wait()` loop with deadline (standard pattern: `for !condition && !timeout && !closed { cond.Wait() }`)
   - If closed and no data: unlock, return `io.EOF`

### Eviction Strategy

- **Soft limit** (`maxBytes`): Only evict chunks that ALL readers have consumed (`chunk.seq < min(all reader seqs)`)
- **Hard limit** (`maxBytes * 2`): Force evict oldest chunk regardless of reader progress. Slow readers lose data.
- When a reader's data is lost, its next `Read` returns `ErrDataLost` once, then the reader's seq is updated to `tail`'s seq to resume.

### Reader Lifecycle

- `NewReader()`: assigns unique ID, sets `reader.seq = b.seq - 1` (will read all future + existing data up to eviction)
- `Unregister(id)`: removes from `readers` map. After unregister, chunks only held by this reader become eligible for eviction.
- `Close()`: sets `closed = true`, calls `cond.Broadcast()` to wake all waiting readers.

## Session Layer Changes

### Multi-Reader Integration

- `Session` gains a `readerIDs map[string]int` mapping MCP client ID to buffer reader ID
- `Session.ReadOutput(readerID int, timeout time.Duration)` passes reader ID to buffer
- `Session.RegisterReader(clientID string) int` creates and caches a reader
- `Session.UnregisterReader(clientID string)` cleans up

### Concurrency Fixes

| Bug | Fix |
|-----|-----|
| `ResizePty` modifies Rows/Cols under RLock | Use full write lock (`Lock()` instead of `RLock()`) |
| `SendInput` stdin write not protected | Add `stdinMu sync.Mutex`, wrap `Stdin.Write` calls |
| `Terminate` lock gap allows double-terminate | Add `terminateOnce sync.Once`, guard entire terminate path |
| `has_more` hardcoded false | Call `buf.HasMore(readerID)` for real value |

### MCP Handler Changes

- `handleStartProcess`: registers temporary reader for initial read, unregisters after
- `handleReadOutput`: uses caller's reader ID, returns real `has_more`
- `handleSendAndRead`: uses caller's reader ID, returns real `has_more`
- Reader ID is passed through MCP tool call context (mapped from client connection)

## Testing Strategy

1. **Single-reader correctness**: existing tests adapted to register/unregister reader
2. **Multi-reader independence**: two readers, write data, verify each reads independently
3. **Soft eviction**: all readers consume -> chunks evicted
4. **Hard eviction**: force evict -> slow reader gets `ErrDataLost`
5. **Race detection**: `go test -race` passes
6. **Reader lifecycle**: unregister frees memory, eviction respects remaining readers
7. **Concurrency regression**: ResizePty, SendInput, Terminate under concurrent access
8. **Benchmark**: compare throughput of old vs new implementation

## Error Types

```go
var ErrClosed   = errors.New("buffer: closed")
var ErrDataLost = errors.New("buffer: data lost due to slow consumption")
var ErrReader   = errors.New("buffer: invalid reader ID")
```
