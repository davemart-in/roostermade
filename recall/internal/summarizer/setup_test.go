package summarizer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsValidProvider(t *testing.T) {
	valid := []string{ProviderClaude, ProviderCodex, ProviderCursor, ProviderNone}
	for _, provider := range valid {
		if !IsValidProvider(provider) {
			t.Fatalf("expected provider %q to be valid", provider)
		}
	}
	if IsValidProvider("unknown") {
		t.Fatal("expected unknown provider to be invalid")
	}
}

func TestWriteWrapperCreatesExecutableScript(t *testing.T) {
	projectRoot := t.TempDir()

	path, err := WriteWrapper(projectRoot, ProviderCodex)
	if err != nil {
		t.Fatalf("write wrapper: %v", err)
	}

	if filepath.Base(path) != "summarize-codex.sh" {
		t.Fatalf("unexpected wrapper name: %s", path)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat wrapper: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("expected mode 0755, got %o", info.Mode().Perm())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read wrapper: %v", err)
	}
	if !strings.Contains(string(data), "codex exec") {
		t.Fatalf("expected codex wrapper content, got %q", string(data))
	}
}

func TestRecommendedProviderFallsBackToNone(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("CURSOR_API_KEY", "")

	if got := RecommendedProvider(); got != ProviderNone {
		t.Fatalf("expected provider %q, got %q", ProviderNone, got)
	}
}
