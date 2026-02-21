package cli

import (
	"strings"
	"testing"

	"github.com/roostermade/recall/internal/summarizer"
)

func TestResolveEffectiveSummarizerCmd(t *testing.T) {
	t.Setenv(summarizerCmdEnvVar, "")
	if got := resolveEffectiveSummarizerCmd("  /tmp/cmd  "); got != "/tmp/cmd" {
		t.Fatalf("unexpected configured command: %q", got)
	}

	t.Setenv(summarizerCmdEnvVar, "echo env")
	if got := resolveEffectiveSummarizerCmd("/tmp/cmd"); got != "echo env" {
		t.Fatalf("expected env override, got %q", got)
	}
}

func TestLooksPathLike(t *testing.T) {
	if !looksPathLike("./bin/cmd") {
		t.Fatal("expected relative path-like command")
	}
	if !looksPathLike("/usr/local/bin/cmd") {
		t.Fatal("expected absolute path-like command")
	}
	if looksPathLike("codex exec") {
		t.Fatal("did not expect shell command to be path-like")
	}
}

func TestProviderChecksNoneAndUnknown(t *testing.T) {
	checks := providerChecks(summarizer.ProviderNone)
	if len(checks) == 0 || checks[0].Level != doctorWarn {
		t.Fatalf("expected warn checks for none provider, got %#v", checks)
	}

	checks = providerChecks("mystery")
	if len(checks) == 0 {
		t.Fatal("expected checks for unknown provider")
	}
	if checks[0].Level != doctorWarn || !strings.Contains(checks[0].Message, "unknown provider") {
		t.Fatalf("unexpected unknown provider check: %#v", checks[0])
	}
}
