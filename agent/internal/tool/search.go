package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SearchConfig holds options for the web_search tool.
type SearchConfig struct {
	// APIKey is the Tavily API key. Set via config or GO_AGENT_SEARCH_API_KEY env var.
	APIKey string
	// MaxResults controls how many results are returned (default: 5).
	MaxResults int
}

// NewWebSearchTool returns a tool that searches the web via the Tavily API.
// If APIKey is empty the tool returns a clear error rather than panicking at startup.
func NewWebSearchTool(cfg SearchConfig) Tool {
	if cfg.MaxResults == 0 {
		cfg.MaxResults = 5
	}

	return Tool{
		Name:        "web_search",
		Description: "Search the web and return the top results with titles, URLs, and content snippets. Use for current events, factual lookups, or information beyond training data.",
		Parameters:  []byte(`{"type":"object","properties":{"query":{"type":"string","description":"Search query"}},"required":["query"]}`),
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			var p struct {
				Query string `json:"query"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return "", err
			}
			if strings.TrimSpace(p.Query) == "" {
				return "", fmt.Errorf("query is empty")
			}
			return tavilySearch(ctx, cfg.APIKey, p.Query, cfg.MaxResults)
		},
	}
}

func tavilySearch(ctx context.Context, apiKey, query string, maxResults int) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("web_search is not configured: set search.api_key in config.yaml or GO_AGENT_SEARCH_API_KEY env var")
	}

	reqBody, _ := json.Marshal(map[string]any{
		"api_key":      apiKey,
		"query":        query,
		"max_results":  maxResults,
		"search_depth": "basic",
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("search API returned HTTP %d", resp.StatusCode)
	}

	var result struct {
		Answer  string `json:"answer"`
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse search response: %w", err)
	}

	var sb strings.Builder
	if result.Answer != "" {
		sb.WriteString("Summary: ")
		sb.WriteString(result.Answer)
		sb.WriteString("\n\n")
	}
	for i, r := range result.Results {
		fmt.Fprintf(&sb, "[%d] %s\n%s\n%s\n\n", i+1, r.Title, r.URL, r.Content)
	}
	return strings.TrimSpace(sb.String()), nil
}
