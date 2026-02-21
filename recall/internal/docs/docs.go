package docs

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/roostermade/recall/internal/config"
)

var knownDocTitles = map[string]string{
	"project-overview": "Project Overview",
	"architecture":     "Architecture",
	"tech-stack":       "Tech Stack",
	"design":           "Design",
	"api":              "API",
	"mcp":              "MCP",
	"auth":             "Auth",
	"principles":       "Principles",
}
var knownDocOrder = []string{
	"project-overview",
	"architecture",
	"tech-stack",
	"design",
	"api",
	"mcp",
	"auth",
	"principles",
}

const ContextFilename = "context.md"

type Entry struct {
	Filename   string
	ModifiedAt *time.Time
	Missing    bool
}

func NormalizeDocName(input string) (filename string, base string, err error) {
	normalized := strings.TrimSpace(input)
	if normalized == "" {
		return "", "", errors.New("doc name cannot be empty")
	}

	normalized = strings.ReplaceAll(normalized, "\\", "/")
	if strings.Contains(normalized, "/") || strings.Contains(normalized, "..") {
		return "", "", errors.New("doc name cannot include path separators")
	}

	if strings.HasSuffix(strings.ToLower(normalized), ".md") {
		normalized = normalized[:len(normalized)-3]
	}

	normalized = strings.ToLower(normalized)
	normalized = strings.ReplaceAll(normalized, "_", "-")
	normalized = strings.ReplaceAll(normalized, " ", "-")
	normalized = collapseHyphens(normalized)
	normalized = strings.Trim(normalized, "-")
	if normalized == "" {
		return "", "", errors.New("doc name cannot be empty")
	}

	for _, r := range normalized {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return "", "", errors.New("doc name must only include letters, numbers, spaces, underscores, or hyphens")
	}

	return normalized + ".md", normalized, nil
}

func IsKnown(base string) bool {
	_, ok := knownDocTitles[base]
	return ok
}

func TitleFor(base string) string {
	if title, ok := knownDocTitles[base]; ok {
		return title
	}
	return base
}

func KnownDocBases() []string {
	out := make([]string, len(knownDocOrder))
	copy(out, knownDocOrder)
	return out
}

func TemplateFor(base string) string {
	title, ok := knownDocTitles[base]
	if !ok {
		return ""
	}

	return "# " + title + "\n\nDescribe this area for future agent context.\n"
}

func DocPath(projectRoot, filename string) string {
	return filepath.Join(config.DirPath(projectRoot), filename)
}

func EnsureDocFile(projectRoot, filename, content string) (bool, error) {
	path := DocPath(projectRoot, filename)
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, err
	}

	return true, nil
}

func RegisterDoc(cfg *config.Config, filename string) bool {
	for _, existing := range cfg.Docs {
		if existing == filename {
			return false
		}
	}

	cfg.Docs = append(cfg.Docs, filename)
	sort.Strings(cfg.Docs)
	return true
}

func ListRegistered(projectRoot string, cfg config.Config) ([]Entry, error) {
	if len(cfg.Docs) == 0 {
		return []Entry{}, nil
	}

	entries := make([]Entry, 0, len(cfg.Docs))
	for _, filename := range cfg.Docs {
		path := DocPath(projectRoot, filename)
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				entries = append(entries, Entry{
					Filename: filename,
					Missing:  true,
				})
				continue
			}
			return nil, err
		}

		modifiedAt := info.ModTime()
		entries = append(entries, Entry{
			Filename:   filename,
			ModifiedAt: &modifiedAt,
		})
	}

	return entries, nil
}

func collapseHyphens(v string) string {
	parts := strings.Split(v, "-")
	trimmed := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		trimmed = append(trimmed, p)
	}
	return strings.Join(trimmed, "-")
}
