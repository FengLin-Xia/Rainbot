package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// ShellExecConfig holds options for the shell_exec tool.
type ShellExecConfig struct {
	// WorkDir is the working directory for commands. Empty means inherit the server's cwd.
	WorkDir string
}

// NewShellExecTool returns a tool that runs arbitrary bash commands.
// stdout and stderr are merged; non-zero exit codes are surfaced in the output
// rather than as errors so the agent can read the failure message.
func NewShellExecTool(cfg ShellExecConfig) Tool {
	return Tool{
		Name:        "shell_exec",
		Description: "Execute a shell command and return its combined stdout+stderr. Non-zero exit codes are included in the output so you can diagnose failures.",
		Parameters:  []byte(`{"type":"object","properties":{"command":{"type":"string","description":"The bash command to execute"}},"required":["command"]}`),
		Handler: func(ctx context.Context, params json.RawMessage) (string, error) {
			var p struct {
				Command string `json:"command"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return "", err
			}
			if strings.TrimSpace(p.Command) == "" {
				return "", fmt.Errorf("command is empty")
			}

			cmd := exec.CommandContext(ctx, "bash", "-c", p.Command)
			if cfg.WorkDir != "" {
				cmd.Dir = cfg.WorkDir
			}

			var buf bytes.Buffer
			cmd.Stdout = &buf
			cmd.Stderr = &buf

			err := cmd.Run()
			out := buf.String()

			if err != nil {
				if out != "" {
					return out + fmt.Sprintf("\n[exit: %v]", err), nil
				}
				return fmt.Sprintf("[exit: %v]", err), nil
			}
			return out, nil
		},
	}
}
