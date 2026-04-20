# Go Agent Runtime

A minimal, stable Go agent runtime with streaming tool execution and a pluggable style post-processing layer.

## Quick Start

**First time — set up your API key:**

```bash
make init-config
# then edit ~/.config/go-agent/config.yaml and fill in your keys
```

**Run:**

```bash
make run
```

**Build binary:**

```bash
make build
./server
```

## Configuration

Configuration is layered (lowest → highest priority):

| Source | Path | Notes |
|--------|------|-------|
| Project config | `configs/config.yaml` | Safe to commit — no keys |
| User config | `~/.config/go-agent/config.yaml` | Your API keys, never in git |
| Env vars | `GO_AGENT_LLM_API_KEY`, `GO_AGENT_SEARCH_API_KEY` | Highest priority |

Minimal user config:

```yaml
llm:
  api_key: "sk-xxx"

search:
  api_key: "tvly-xxx"   # Tavily, for web_search tool
```

## Project Config Reference

```yaml
server:
  addr: ":8081"

llm:
  provider: openai_compat   # openai | openai_compat | ollama | anthropic
  model: deepseek-chat
  base_url: "https://api.deepseek.com/v1"

style:
  enabled: false
  backend: prompt_rewriter  # prompt_rewriter | local_model

tool:
  timeout_s: 30
  work_dir: ""    # working directory for shell_exec
  base_dir: ""    # restrict file_read to this path; empty = unrestricted

skills:
  dir: "skills"   # ClawHub skill bundles directory; empty = disabled

storage:
  session_db_path: ""   # empty = in-memory; set path to enable BoltDB persistence

memory:
  short_term_max: 50
```

## Built-in Tools

| Name | Description |
|------|-------------|
| `echo` | Echoes input back — useful for pipeline smoke tests |
| `shell_exec` | Runs a bash command, returns stdout+stderr |
| `file_read` | Reads a local file (optionally restricted to `base_dir`) |
| `web_search` | Web search via Tavily API (requires `search.api_key`) |

## ClawHub Skills

Drop any [ClawHub](https://clawhub.ai) skill bundle into `agent/skills/`:

```
agent/skills/
└── my-skill/
    └── SKILL.md
```

The loader runs at startup and:

- Injects the skill's markdown body into the system prompt
- Registers CLI-backed skills as callable tools (if `bins` declared in frontmatter)
- Warns on missing binaries or unset env vars, but does not block startup

Skills with CLI dependencies must have those binaries installed manually — the loader does not auto-install them.

## HTTP API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `POST` | `/sessions` | Create a new session |
| `GET` | `/sessions/:id` | Get session info |
| `DELETE` | `/sessions/:id` | Delete a session |
| `POST` | `/sessions/:id/turns` | Send a message (SSE streaming) |
| `GET` | `/metrics` | Last 50 turn metrics (JSON) |

### SSE Event Types

```
event: text        — streamed text chunk
event: tool_start  — tool call started
event: tool_done   — tool call completed
event: error       — turn error
event: done        — turn complete
```

## Architecture

```
User Input
→ Session / Prompt Builder
→ LLM (streaming, agentic tool loop)
→ Structured Response (JSON extraction via second LLM call)
→ Style Processor (optional post-processing layer)
→ SSE stream output
```

- **Runtime stability first** — turn lifecycle, concurrent tool execution, retry, timeouts
- **Style as a post-processor** — never replans, never calls tools, bypassed on high-risk scenes
- **Observable** — first-token latency, tool latency, style latency, fallback reasons logged per turn

## Development

```bash
make test    # run all tests
make vet     # static analysis
make clean   # remove built binary
```
