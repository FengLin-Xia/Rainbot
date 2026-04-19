package runtime

import (
	"fmt"
	"sync"
	"time"

	"github.com/xia-rain/go_agent/internal/llm"
	"github.com/xia-rain/go_agent/internal/memory"
)

// Session represents a single user conversation context.
type Session struct {
	ID        string
	CreatedAt time.Time

	mu      sync.Mutex
	history *memory.ShortTermMemory
	summary *memory.SummaryMemory
}

func NewSession(id string) *Session {
	return &Session{
		ID:        id,
		CreatedAt: time.Now(),
		history:   memory.NewShortTerm(50),
		summary:   memory.NewSummaryMemory(),
	}
}

func (s *Session) AppendHistory(msg llm.Message)  { s.history.Append(msg) }
func (s *Session) GetHistory() []llm.Message      { return s.history.All() }
func (s *Session) GetSummary() string             { return s.summary.Get() }
func (s *Session) SetSummary(summary string)      { s.summary.Set(summary) }

// SessionStore manages active sessions with optional bolt persistence.
type SessionStore struct {
	mu        sync.RWMutex
	sessions  map[string]*Session
	persister *BoltPersister // nil = memory-only
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]*Session)}
}

// NewPersistentSessionStore opens (or creates) a bolt DB at path and loads
// any previously saved sessions into memory.
func NewPersistentSessionStore(path string) (*SessionStore, error) {
	p, err := NewBoltPersister(path)
	if err != nil {
		return nil, err
	}
	snaps, err := p.loadAll()
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("load sessions: %w", err)
	}
	s := &SessionStore{
		sessions:  make(map[string]*Session, len(snaps)),
		persister: p,
	}
	for _, snap := range snaps {
		s.sessions[snap.ID] = sessionFromSnapshot(snap)
	}
	return s, nil
}

func (s *SessionStore) Create(id string) *Session {
	sess := NewSession(id)
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	if s.persister != nil {
		_ = s.persister.save(sessionToSnapshot(sess))
	}
	return sess
}

func (s *SessionStore) Get(id string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

func (s *SessionStore) Delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
	if s.persister != nil {
		_ = s.persister.delete(id)
	}
}

// Persist writes the current in-memory state of session id to the bolt DB.
// No-op when persistence is disabled.
func (s *SessionStore) Persist(id string) {
	if s.persister == nil {
		return
	}
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok {
		return
	}
	_ = s.persister.save(sessionToSnapshot(sess))
}

// Close shuts down the bolt DB if persistence is enabled.
func (s *SessionStore) Close() error {
	if s.persister != nil {
		return s.persister.Close()
	}
	return nil
}
