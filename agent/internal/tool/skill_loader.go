package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/xia-rain/go_agent/internal/obs"
)

// skillFrontmatter represents the YAML header of a SKILL.md file.
type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
	Metadata    struct {
		OpenClaw struct {
			Requires struct {
				Env  []string `yaml:"env"`
				Bins []string `yaml:"bins"`
			} `yaml:"requires"`
			PrimaryEnv string `yaml:"primaryEnv"`
		} `yaml:"openclaw"`
	} `yaml:"metadata"`
}

// SkillLoadResult holds the output of LoadSkillsDir.
type SkillLoadResult struct {
	// Tools are CLI-backed skills ready to register in the tool Registry.
	Tools []Tool
	// SkillPrompts are markdown bodies to inject into the system prompt.
	SkillPrompts []string
	// Warnings collects non-fatal issues (missing bins, unset env vars).
	Warnings []string
}

// LoadSkillsDir scans dir for ClawHub skill bundles. Each subdirectory that
// contains a SKILL.md is loaded. Skills with declared CLI binaries are
// registered as callable tools; all skills inject their body into the system
// prompt so the agent knows how to use them.
//
// Returns an empty result (no error) if dir does not exist.
func LoadSkillsDir(ctx context.Context, dir string) (*SkillLoadResult, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return &SkillLoadResult{}, nil
		}
		return nil, fmt.Errorf("read skills dir %q: %w", dir, err)
	}

	result := &SkillLoadResult{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := findSkillMD(filepath.Join(dir, entry.Name()))
		if skillPath == "" {
			continue
		}

		fm, body, err := parseSkillMD(skillPath)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("skill %s: parse error: %v", entry.Name(), err))
			continue
		}

		name := fm.Name
		if name == "" {
			name = entry.Name()
		}

		for _, envVar := range fm.Metadata.OpenClaw.Requires.Env {
			if os.Getenv(envVar) == "" {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("skill %s: env var %s not set", name, envVar))
			}
		}

		bins := fm.Metadata.OpenClaw.Requires.Bins
		if len(bins) > 0 {
			if missing := missingBins(bins); len(missing) > 0 {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("skill %s: binaries not found in PATH: %s", name, strings.Join(missing, ", ")))
			}
			result.Tools = append(result.Tools, buildCLITool(name, fm.Description, bins[0]))
		}

		if body != "" {
			obs.Info(ctx, "skill_loaded", "name", name, "has_cli", len(bins) > 0)
			result.SkillPrompts = append(result.SkillPrompts,
				fmt.Sprintf("### Skill: %s\n%s", name, body))
		}
	}
	return result, nil
}

// BuildSkillSystemPrompt appends loaded skill bodies to a base system prompt.
// Returns base unchanged when there are no skill prompts.
func BuildSkillSystemPrompt(base string, skillPrompts []string) string {
	if len(skillPrompts) == 0 {
		return base
	}
	return base + "\n\n## Installed Skills\n\n" + strings.Join(skillPrompts, "\n\n---\n\n")
}

// findSkillMD returns the path to SKILL.md inside dir (case-insensitive common
// variants). Returns empty string if not found.
func findSkillMD(dir string) string {
	for _, name := range []string{"SKILL.md", "skill.md", "Skill.md"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// parseSkillMD splits a SKILL.md file into its YAML frontmatter and markdown body.
func parseSkillMD(path string) (skillFrontmatter, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return skillFrontmatter{}, "", err
	}

	var fm skillFrontmatter
	body := strings.TrimSpace(string(data))

	// Standard frontmatter: file starts with "---\n", closes with "\n---".
	if strings.HasPrefix(body, "---") {
		rest := body[3:]
		if idx := strings.Index(rest, "\n---"); idx != -1 {
			yamlSrc := rest[:idx]
			afterClose := rest[idx+4:]
			if err := yaml.Unmarshal([]byte(yamlSrc), &fm); err != nil {
				return fm, "", fmt.Errorf("parse frontmatter: %w", err)
			}
			body = strings.TrimSpace(afterClose)
		}
	}

	return fm, body, nil
}

// missingBins returns any bin names from the list that are not found in PATH.
func missingBins(bins []string) []string {
	var missing []string
	for _, bin := range bins {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}
	return missing
}

// buildCLITool creates a Tool that runs primaryBin with agent-provided arguments.
func buildCLITool(name, description, primaryBin string) Tool {
	if description == "" {
		description = fmt.Sprintf("Run the %s CLI tool.", primaryBin)
	}
	params := json.RawMessage(`{"type":"object","properties":{"args":{"type":"string","description":"Arguments to pass after the binary name (may be empty for default invocation)"}},"required":["args"]}`)

	return Tool{
		Name:        name,
		Description: description,
		Parameters:  params,
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var p struct {
				Args string `json:"args"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return "", err
			}

			cmdStr := primaryBin
			if a := strings.TrimSpace(p.Args); a != "" {
				cmdStr += " " + a
			}
			obs.Debug(ctx, "skill_exec", "skill", name, "cmd", cmdStr)

			cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)
			var buf bytes.Buffer
			cmd.Stdout = &buf
			cmd.Stderr = &buf

			runErr := cmd.Run()
			out := buf.String()
			if runErr != nil {
				if out != "" {
					return out + fmt.Sprintf("\n[exit: %v]", runErr), nil
				}
				return fmt.Sprintf("[exit: %v]", runErr), nil
			}
			return out, nil
		},
	}
}
