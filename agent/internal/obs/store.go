package obs

import "sync"

// MetricsStore holds the last N TurnMetrics in a ring buffer, thread-safe.
type MetricsStore struct {
	mu      sync.RWMutex
	ring    []TurnMetrics
	maxSize int
	head    int
	count   int
}

func NewMetricsStore(maxSize int) *MetricsStore {
	if maxSize <= 0 {
		maxSize = 200
	}
	return &MetricsStore{ring: make([]TurnMetrics, maxSize), maxSize: maxSize}
}

func (s *MetricsStore) Push(m TurnMetrics) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ring[s.head] = m
	s.head = (s.head + 1) % s.maxSize
	if s.count < s.maxSize {
		s.count++
	}
}

// Recent returns up to n entries, newest first.
func (s *MetricsStore) Recent(n int) []TurnMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if n <= 0 || n > s.count {
		n = s.count
	}
	out := make([]TurnMetrics, n)
	for i := 0; i < n; i++ {
		idx := ((s.head - 1 - i) + s.maxSize) % s.maxSize
		out[i] = s.ring[idx]
	}
	return out
}

var defaultStore = NewMetricsStore(200)

func DefaultMetricsStore() *MetricsStore { return defaultStore }
