package memory

import (
	"sync"

	"github.com/xia-rain/go_agent/internal/llm"
)

// ShortTermMemory holds the in-session conversation history.
type ShortTermMemory struct {
	mu       sync.RWMutex
	messages []llm.Message
	maxSize  int
}

func NewShortTerm(maxSize int) *ShortTermMemory {
	if maxSize <= 0 {
		maxSize = 50
	}
	return &ShortTermMemory{maxSize: maxSize}
}

func (m *ShortTermMemory) Append(msg llm.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
	// Trim oldest messages when limit is exceeded, but always keep in pairs
	// to avoid orphaned tool_call / tool result messages.
	for len(m.messages) > m.maxSize {
		m.messages = m.messages[2:]
	}
}

func (m *ShortTermMemory) All() []llm.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]llm.Message, len(m.messages))
	copy(out, m.messages)
	return out
}

func (m *ShortTermMemory) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.messages)
}

func (m *ShortTermMemory) MaxSize() int { return m.maxSize }

// DrainOldestIfAbove atomically drains the oldest drainCount messages when
// current length exceeds threshold. Count is rounded down to pairs to avoid
// orphaning tool_call / tool result messages. Returns nil if threshold not met.
func (m *ShortTermMemory) DrainOldestIfAbove(threshold, drainCount int) []llm.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.messages) <= threshold {
		return nil
	}
	n := drainCount - (drainCount % 2)
	if n <= 0 || n > len(m.messages) {
		return nil
	}
	drained := make([]llm.Message, n)
	copy(drained, m.messages[:n])
	m.messages = m.messages[n:]
	return drained
}

func (m *ShortTermMemory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = m.messages[:0]
}
