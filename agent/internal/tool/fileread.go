package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxFileReadBytes = 64 * 1024 // 64 KB

// NewFileReadTool returns a tool that reads file contents.
// baseDir restricts access to that directory tree; pass "" to allow any path.
func NewFileReadTool(baseDir string) Tool {
	var absBase string
	if baseDir != "" {
		absBase, _ = filepath.Abs(baseDir)
	}

	return Tool{
		Name:        "file_read",
		Description: "Read a file and return its contents as text. Files larger than 64 KB are truncated with a notice.",
		Parameters:  []byte(`{"type":"object","properties":{"path":{"type":"string","description":"Path to the file"}},"required":["path"]}`),
		Handler: func(_ context.Context, params json.RawMessage) (string, error) {
			var p struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return "", err
			}

			abs, err := filepath.Abs(p.Path)
			if err != nil {
				return "", fmt.Errorf("invalid path: %w", err)
			}

			if absBase != "" {
				if !strings.HasPrefix(abs, absBase+string(filepath.Separator)) && abs != absBase {
					return "", fmt.Errorf("path %q is outside the allowed directory", p.Path)
				}
			}

			f, err := os.Open(abs)
			if err != nil {
				return "", err
			}
			defer f.Close()

			info, err := f.Stat()
			if err != nil {
				return "", err
			}
			if info.IsDir() {
				return "", fmt.Errorf("%q is a directory", abs)
			}

			data, err := io.ReadAll(io.LimitReader(f, maxFileReadBytes+1))
			if err != nil {
				return "", err
			}

			if len(data) > maxFileReadBytes {
				return string(data[:maxFileReadBytes]) + fmt.Sprintf("\n\n[truncated — file is %d bytes, showing first %d]", info.Size(), maxFileReadBytes), nil
			}
			return string(data), nil
		},
	}
}
