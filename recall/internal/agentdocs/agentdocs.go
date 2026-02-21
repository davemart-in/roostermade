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
			"## Recall Integration\n\n" +
			"Use Recall for persistent project memory across sessions and context compaction.\n" +
			"At session start run `recall context` (loads `.recall/context.md`).\n" +
			"After each successful task, run `recall note add \"<what changed or was decided>\"`.\n" +
			"Use `recall summary list` / `recall summary get <id>` for compressed history.\n" +
			"General tools remain available anytime; if MCP is enabled, Recall MCP tools are available too.\n" +
			"<!-- RECALL_MANAGED_BLOCK_END -->",
	)
}
