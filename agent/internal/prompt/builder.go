package prompt

import (
	"github.com/xia-rain/go_agent/internal/llm"
)

// Builder assembles message slices for LLM calls.
type Builder struct {
	systemPrompt string
	maxHistory   int
}

func New(systemPrompt string, maxHistory int) *Builder {
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}
	if maxHistory <= 0 {
		maxHistory = 20
	}
	return &Builder{systemPrompt: systemPrompt, maxHistory: maxHistory}
}

// Build constructs the message list for a normal agentic turn.
func (b *Builder) Build(history []llm.Message, userInput string) []llm.Message {
	msgs := make([]llm.Message, 0, len(history)+2)
	msgs = append(msgs, llm.Message{Role: llm.RoleSystem, Content: b.systemPrompt})

	// Trim history if needed (keep most recent entries).
	if len(history) > b.maxHistory {
		history = history[len(history)-b.maxHistory:]
	}
	msgs = append(msgs, history...)
	msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: userInput})
	return msgs
}

// BuildStructureRequest asks the model to convert a raw answer into a
// StructuredResponse JSON.
func (b *Builder) BuildStructureRequest(rawAnswer string) []llm.Message {
	return []llm.Message{
		{Role: llm.RoleSystem, Content: StructureSystemPrompt},
		{Role: llm.RoleUser, Content: rawAnswer},
	}
}
