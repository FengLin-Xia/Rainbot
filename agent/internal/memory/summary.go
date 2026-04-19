package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/xia-rain/go_agent/internal/llm"
)

const summarySystemPrompt = `You are a conversation memory compressor.
Summarize the conversation segment below into one concise paragraph (under 150 words).
Focus on: decisions made, key facts established, user intent, topics discussed.
Write in past tense. Be specific about names, numbers, versions, and conclusions.
If a previous summary is provided, merge it with the new segment into a single updated summary.
Output only the summary text — no preamble, no labels.`

// SummaryMemory holds a rolling compressed summary of older conversation turns.
type SummaryMemory struct {
	mu      sync.RWMutex
	summary string
}

func NewSummaryMemory() *SummaryMemory {
	return &SummaryMemory{}
}

func (s *SummaryMemory) Get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.summary
}

func (s *SummaryMemory) Set(summary string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.summary = summary
}

// Compress summarizes msgs and merges the result with any existing summary.
// Tool and system messages are excluded from the transcript sent to the LLM.
func (s *SummaryMemory) Compress(ctx context.Context, msgs []llm.Message, client llm.ModelClient) error {
	if len(msgs) == 0 {
		return nil
	}

	var sb strings.Builder
	existing := s.Get()
	if existing != "" {
		sb.WriteString("Previous summary:\n")
		sb.WriteString(existing)
		sb.WriteString("\n\nNew conversation segment:\n")
	}
	for _, m := range msgs {
		if m.Role == llm.RoleSystem || m.Role == llm.RoleTool {
			continue
		}
		label := m.Role
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		sb.WriteString(label + ": " + content + "\n")
	}

	req := llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: summarySystemPrompt},
			{Role: llm.RoleUser, Content: sb.String()},
		},
		MaxTokens:   300,
		Temperature: 0.3,
	}

	resp, err := client.Generate(ctx, req)
	if err != nil {
		return fmt.Errorf("summary compress: %w", err)
	}

	s.Set(strings.TrimSpace(resp.Message.Content))
	return nil
}
