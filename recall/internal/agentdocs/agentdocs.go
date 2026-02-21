package agentdocs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/roostermade/recall/internal/summarizer"
)

const (
	ClaudeFile = "CLAUDE.md"
	CodexFile  = "AGENTS.md"
	CursorFile = "CURSOR.md"

	blockStartMarker = "<!-- RECALL_MANAGED_BLOCK_START -->"
	blockEndMarker   = "<!-- RECALL_MANAGED_BLOCK_END -->"
)

func TargetFileForProvider(provider string) (string, error) {
	switch provider {
	case summarizer.ProviderClaude:
		return ClaudeFile, nil
	case summarizer.ProviderCodex:
		return CodexFile, nil
	case summarizer.ProviderCursor:
		return CursorFile, nil
	default:
		return "", fmt.Errorf("unsupported provider %q", provider)
	}
}

func FileExists(projectRoot, filename string) (bool, error) {
	path := filepath.Join(projectRoot, filename)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func EnsureRecallBlock(projectRoot, filename string) (string, error) {
	path := filepath.Join(projectRoot, filename)
	block := BuildRecallBlock()

	existing, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}

		initial := "# " + strings.TrimSuffix(filename, filepath.Ext(filename)) + "\n\n" + block + "\n"
		if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
			return "", err
		}
		return "created", nil
	}

	content := string(existing)
	start := strings.Index(content, blockStartMarker)
	end := strings.Index(content, blockEndMarker)

	switch {
	case start >= 0 && end > start:
		end += len(blockEndMarker)
		updated := content[:start] + block + content[end:]
		if updated == content {
			return "unchanged", nil
		}
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return "", err
		}
		return "updated", nil
	default:
		suffix := "\n\n"
		trimmed := strings.TrimRight(content, "\n")
		if trimmed == "" {
			suffix = ""
		}
		updated := trimmed + suffix + block + "\n"
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return "", err
		}
		return "appended", nil
	}
}

func BuildRecallBlock() string {
	return strings.TrimSpace(
		"<!-- RECALL_MANAGED_BLOCK_START -->\n" +
			"## Recall Integration (HIGHLY recommended)\n\n" +
			"Recall is available in this repository and provides persistent project-scoped memory.\n\n" +
			"- Persistent memory is available through Recall (`recall context`, `recall thought add`, summaries, and docs).\n" +
			"- General tools are available and can be used anytime.\n" +
			"- If MCP is enabled for this project, Recall MCP tools are available.\n\n" +
			"HIGHLY recommended workflow:\n" +
			"1. At session start, load memory with `recall context`.\n" +
			"2. After each successful completion of a user prompt/task, record a thought:\n" +
			"   `recall thought add \"<what was completed>\"`\n" +
			"3. Continue using normal tools as needed; Recall can be used at any time.\n" +
			"<!-- RECALL_MANAGED_BLOCK_END -->",
	)
}
