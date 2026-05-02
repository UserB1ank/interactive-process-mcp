package session

import (
	"fmt"
	"sync"
	"time"

	"github.com/mac01/interactive-process-mcp/internal/message"
	"github.com/mac01/interactive-process-mcp/internal/storage"
	"github.com/mac01/interactive-process-mcp/pkg/api"
)

// Manager is a thread-safe registry of sessions with persistence.
type Manager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
	sshAddr  string
	msgMgr   *message.Manager
	store    *storage.Store
}

// NewManager creates a Manager.
func NewManager(sshAddr string, msgMgr *message.Manager, store *storage.Store) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		sshAddr:  sshAddr,
		msgMgr:   msgMgr,
		store:    store,
	}
}

// Create starts a new session and registers it.
func (m *Manager) Create(cfg Config) (*Session, error) {
	s, err := New(m.sshAddr, cfg, m.msgMgr)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()

	m.persist()
	return s, nil
}

// Get returns a session by ID.
func (m *Manager) Get(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// ListAll returns metadata for all sessions.
func (m *Manager) ListAll() []api.Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]api.Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		result = append(result, s.Info())
	}
	return result
}

// Terminate stops a session.
func (m *Manager) Terminate(id string, force bool, gracePeriod float64) {
	m.mu.RLock()
	s := m.sessions[id]
	m.mu.RUnlock()
	if s != nil {
		s.Terminate(force, time.Duration(gracePeriod*float64(time.Second)))
		m.persist()
	}
}

// Delete removes an exited session from the registry.
// Returns error if the session is still running.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	s := m.sessions[id]
	if s == nil {
		m.mu.Unlock()
		return nil
	}
	if s.Info().Status == api.SessionRunning {
		m.mu.Unlock()
		return fmt.Errorf("cannot delete running session %q, terminate it first", id)
	}
	delete(m.sessions, id)
	m.mu.Unlock()
	m.persist()
	return nil
}

// CleanupAll terminates all running sessions.
func (m *Manager) CleanupAll(force bool) {
	m.mu.RLock()
	list := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	m.mu.RUnlock()
	for _, s := range list {
		s.Terminate(force, 0)
	}
	m.persist()
}

func (m *Manager) persist() {
	if m.store == nil {
		return
	}
	_ = m.store.SaveSessions(m.ListAll())
}
