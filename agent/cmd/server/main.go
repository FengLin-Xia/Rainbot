package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/xia-rain/go_agent/api"
	"github.com/xia-rain/go_agent/internal/llm"
	"github.com/xia-rain/go_agent/internal/obs"
	"github.com/xia-rain/go_agent/internal/prompt"
	"github.com/xia-rain/go_agent/internal/runtime"
	"github.com/xia-rain/go_agent/internal/style"
	"github.com/xia-rain/go_agent/internal/tool"
)

// Config mirrors configs/config.yaml.
type Config struct {
	Server struct {
		Addr          string `yaml:"addr"`
		ReadTimeoutS  int    `yaml:"read_timeout_s"`
		WriteTimeoutS int    `yaml:"write_timeout_s"`
		IdleTimeoutS  int    `yaml:"idle_timeout_s"`
	} `yaml:"server"`
	LLM struct {
		Provider    string  `yaml:"provider"`
		Model       string  `yaml:"model"`
		APIKey      string  `yaml:"api_key"`
		BaseURL     string  `yaml:"base_url"`
		MaxTokens   int     `yaml:"max_tokens"`
		Temperature float32 `yaml:"temperature"`
	} `yaml:"llm"`
	Style struct {
		Enabled      bool   `yaml:"enabled"`
		Backend      string `yaml:"backend"`
		LocalBaseURL string `yaml:"local_base_url"`
		LocalModel   string `yaml:"local_model"`
	} `yaml:"style"`
	Tool struct {
		TimeoutS int    `yaml:"timeout_s"`
		WorkDir  string `yaml:"work_dir"`
		BaseDir  string `yaml:"base_dir"`
	} `yaml:"tool"`
	Search struct {
		APIKey     string `yaml:"api_key"`
		MaxResults int    `yaml:"max_results"`
	} `yaml:"search"`
	Memory struct {
		ShortTermMax int `yaml:"short_term_max"`
	} `yaml:"memory"`
	Storage struct {
		SessionDBPath string `yaml:"session_db_path"`
	} `yaml:"storage"`
}

func main() {
	ctx := context.Background()

	cfg, err := loadConfig("configs/config.yaml")
	if err != nil {
		obs.Warn(ctx, "config_load_failed", "error", err.Error())
		cfg = defaultConfig()
	}

	// Allow API key override via environment variable.
	if key := os.Getenv("GO_AGENT_LLM_API_KEY"); key != "" {
		cfg.LLM.APIKey = key
	}

	// ── Build LLM client ───────────────────────────────────────────────────
	bigModel, err := buildLLMClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: build llm client: %v\n", err)
		os.Exit(1)
	}
	obs.Info(ctx, "llm_ready", "provider", bigModel.Name())

	// ── Build tool registry & executor ────────────────────────────────────
	registry := tool.NewRegistry()
	registerBuiltinTools(registry, cfg)
	executor := tool.NewExecutor(registry, time.Duration(cfg.Tool.TimeoutS)*time.Second)

	// ── Build style processor ──────────────────────────────────────────────
	var styleProc style.Processor
	if cfg.Style.Enabled {
		styleProc = buildStyleProcessor(cfg, bigModel)
		obs.Info(ctx, "style_ready", "backend", cfg.Style.Backend)
	}

	// ── Build engine ───────────────────────────────────────────────────────
	promptBuilder := prompt.New("", 0)
	engine := runtime.NewEngine(runtime.EngineConfig{
		LLM:            bigModel,
		StyleProcessor: styleProc,
		Tools:          executor,
		Registry:       registry,
		Prompt:         promptBuilder,
		MaxTokens:      cfg.LLM.MaxTokens,
		Temperature:    cfg.LLM.Temperature,
	})

	store, err := buildSessionStore(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: session store: %v\n", err)
		os.Exit(1)
	}
	handler := api.NewHandler(engine, store, obs.DefaultMetricsStore())

	// ── Start HTTP server ──────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      handler,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutS) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutS) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeoutS) * time.Second,
	}

	go func() {
		obs.Info(ctx, "server_start", "addr", cfg.Server.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	obs.Info(ctx, "server_shutdown")
	shutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	_ = store.Close()
}

func buildSessionStore(ctx context.Context, cfg *Config) (*runtime.SessionStore, error) {
	if cfg.Storage.SessionDBPath == "" {
		obs.Info(ctx, "session_store", "backend", "memory")
		return runtime.NewSessionStore(), nil
	}
	store, err := runtime.NewPersistentSessionStore(cfg.Storage.SessionDBPath)
	if err != nil {
		return nil, err
	}
	obs.Info(ctx, "session_store", "backend", "bolt", "path", cfg.Storage.SessionDBPath)
	return store, nil
}

func defaultConfig() *Config {
	var cfg Config
	cfg.Server.Addr = ":8080"
	cfg.Server.ReadTimeoutS = 30
	cfg.Server.WriteTimeoutS = 120
	cfg.Server.IdleTimeoutS = 60
	cfg.LLM.Provider = "openai"
	cfg.LLM.Model = "gpt-4o"
	cfg.LLM.MaxTokens = 4096
	cfg.LLM.Temperature = 0.7
	cfg.Style.Enabled = true
	cfg.Style.Backend = "prompt_rewriter"
	cfg.Tool.TimeoutS = 30
	cfg.Memory.ShortTermMax = 50
	return &cfg
}

func loadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cfg Config
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func buildLLMClient(cfg *Config) (llm.ModelClient, error) {
	switch cfg.LLM.Provider {
	case "openai":
		return llm.NewOpenAIClient(cfg.LLM.APIKey, cfg.LLM.Model), nil
	case "openai_compat":
		return llm.NewOpenAICompatClient(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model), nil
	case "ollama":
		base := cfg.LLM.BaseURL
		if base == "" {
			base = "http://localhost:11434"
		}
		return llm.NewOllamaClient(base, cfg.LLM.Model), nil
	case "anthropic":
		return llm.NewAnthropicClient(cfg.LLM.APIKey, cfg.LLM.Model)
	default:
		return nil, fmt.Errorf("unknown provider: %q", cfg.LLM.Provider)
	}
}

func buildStyleProcessor(cfg *Config, bigModel llm.ModelClient) style.Processor {
	switch cfg.Style.Backend {
	case "local_model":
		return style.NewLocalModelRewriter(cfg.Style.LocalBaseURL, cfg.Style.LocalModel)
	default: // "prompt_rewriter"
		return style.NewPromptRewriter(bigModel)
	}
}

// registerBuiltinTools wires the built-in tool implementations.
func registerBuiltinTools(r *tool.Registry, cfg *Config) {
	r.MustRegister(tool.Tool{
		Name:        "echo",
		Description: "Echoes the input back. Useful for testing the tool pipeline.",
		Parameters:  []byte(`{"type":"object","properties":{"text":{"type":"string","description":"Text to echo"}},"required":["text"]}`),
		Handler: func(_ context.Context, params json.RawMessage) (string, error) {
			var p struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return "", err
			}
			return p.Text, nil
		},
	})

	r.MustRegister(tool.NewShellExecTool(tool.ShellExecConfig{
		WorkDir: cfg.Tool.WorkDir,
	}))

	r.MustRegister(tool.NewFileReadTool(cfg.Tool.BaseDir))

	searchAPIKey := cfg.Search.APIKey
	if key := os.Getenv("GO_AGENT_SEARCH_API_KEY"); key != "" {
		searchAPIKey = key
	}
	r.MustRegister(tool.NewWebSearchTool(tool.SearchConfig{
		APIKey:     searchAPIKey,
		MaxResults: cfg.Search.MaxResults,
	}))
}
