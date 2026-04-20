BIN        := server
GO         := go
USER_CFG   := $(HOME)/.config/go-agent/config.yaml

.PHONY: run build test vet clean init-config

run:
	$(GO) run ./cmd/server/

build:
	$(GO) build -o $(BIN) ./cmd/server/

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

clean:
	rm -f $(BIN)

# Create ~/.config/go-agent/config.yaml with API key placeholders.
# Run once: make init-config, then fill in your keys.
init-config:
	@mkdir -p $(HOME)/.config/go-agent
	@if [ -f "$(USER_CFG)" ]; then \
		echo "$(USER_CFG) already exists, skipping."; \
	else \
		printf 'llm:\n  api_key: ""\n\nsearch:\n  api_key: ""\n' > $(USER_CFG); \
		echo "Created $(USER_CFG) — fill in your API keys, then run: make run"; \
	fi
