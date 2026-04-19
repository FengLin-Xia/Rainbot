package runtime

import (
	"sync"
	"time"

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

// SessionStore manages active sessions.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]*Session)}
}

func (s *SessionStore) Create(id string) *Session {
	sess := NewSession(id)
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
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
}
