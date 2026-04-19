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

func (m *ShortTermMemory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = m.messages[:0]
}
