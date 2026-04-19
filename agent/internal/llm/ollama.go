package llm

// NewOllamaClient creates a client for a local Ollama instance.
// Ollama exposes an OpenAI-compatible API at /v1.
func NewOllamaClient(baseURL, model string) *OpenAIClient {
	return NewOpenAICompatClient(baseURL+"/v1", "ollama", model)
}
