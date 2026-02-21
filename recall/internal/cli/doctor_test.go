package cli

import (
	"strings"
	"testing"
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

func TestProviderChecksForCommand(t *testing.T) {
	checks := providerChecksForCommand("")
	if len(checks) == 0 || checks[0].Level != doctorWarn {
		t.Fatalf("expected warn checks for unknown provider, got %#v", checks)
	}

	checks = providerChecksForCommand("codex exec 'hi'")
	if len(checks) == 0 || !strings.Contains(checks[0].Subject, "provider:codex") {
		t.Fatalf("expected codex checks, got %#v", checks)
	}

	if got := inferProviderFromCommand("/tmp/bin/summarize-cursor.sh"); got != "cursor" {
		t.Fatalf("unexpected inferred provider: %q", got)
	}
}
