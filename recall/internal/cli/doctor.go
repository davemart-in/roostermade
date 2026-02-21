package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/roostermade/recall/internal/bootstrap"
	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/db"
	"github.com/roostermade/recall/internal/docs"
	"github.com/spf13/cobra"
)

const summarizerCmdEnvVar = "RECALL_SUMMARIZER_CMD"

type doctorLevel string

const (
	doctorOK   doctorLevel = "OK"
	doctorWarn doctorLevel = "WARN"
	doctorFail doctorLevel = "FAIL"
)

type doctorCheck struct {
	Level   doctorLevel
	Subject string
	Message string
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run environment and project health checks",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			results, hasFailure := runDoctorChecks(cwd)
			for _, result := range results {
				cmd.Printf("[%s] %s: %s\n", result.Level, result.Subject, result.Message)
			}
			if hasFailure {
				return errors.New("doctor found one or more failures")
			}
			return nil
		},
	}
}

func runDoctorChecks(projectRoot string) ([]doctorCheck, bool) {
	checks := make([]doctorCheck, 0)
	hasFailure := false
	add := func(level doctorLevel, subject string, message string) {
		checks = append(checks, doctorCheck{Level: level, Subject: subject, Message: message})
		if level == doctorFail {
			hasFailure = true
		}
	}

	initialized, err := bootstrap.IsInitialized(projectRoot)
	if err != nil {
		add(doctorFail, "project", err.Error())
		return checks, hasFailure
	}
	if !initialized {
		add(doctorFail, "project", "Recall is not initialized (run `recall init`)")
		return checks, hasFailure
	}
	add(doctorOK, "project", "Recall is initialized")

	cfg, err := config.Load(config.ConfigPath(projectRoot))
	if err != nil {
		add(doctorFail, "config", fmt.Sprintf("load failed: %v", err))
		return checks, hasFailure
	}
	add(doctorOK, "config", "loaded .recall/config.json")

	contextPath := docs.DocPath(projectRoot, docs.ContextFilename)
	if _, err := os.Stat(contextPath); err != nil {
		if os.IsNotExist(err) {
			add(doctorWarn, "context", ".recall/context.md is missing")
		} else {
			add(doctorFail, "context", err.Error())
		}
	} else {
		add(doctorOK, "context", ".recall/context.md exists")
	}

	conn, err := db.Open(config.DBPath(projectRoot))
	if err != nil {
		add(doctorFail, "database", fmt.Sprintf("open failed: %v", err))
	} else {
		_ = conn.Close()
		add(doctorOK, "database", ".recall/recall.db is readable")
	}

	effectiveCmd := resolveEffectiveSummarizerCmd(cfg.SummarizerCmd)
	if effectiveCmd == "" {
		add(doctorWarn, "summarizer_cmd", "not configured (auto-summarization disabled)")
	} else {
		add(doctorOK, "summarizer_cmd", fmt.Sprintf("configured: %s", effectiveCmd))
		if looksPathLike(effectiveCmd) {
			if err := checkExecutablePath(effectiveCmd, projectRoot); err != nil {
				add(doctorFail, "summarizer_cmd", err.Error())
			} else {
				add(doctorOK, "summarizer_cmd", "command path is executable")
			}
		} else {
			add(doctorWarn, "summarizer_cmd", "shell command resolution cannot be fully validated")
		}
	}

	if effectiveCmd != "" {
		for _, providerCheck := range providerChecksForCommand(effectiveCmd) {
			add(providerCheck.Level, providerCheck.Subject, providerCheck.Message)
		}
	}

	return checks, hasFailure
}

func resolveEffectiveSummarizerCmd(configuredCmd string) string {
	env := strings.TrimSpace(os.Getenv(summarizerCmdEnvVar))
	if env != "" {
		return env
	}
	return strings.TrimSpace(configuredCmd)
}

func providerChecksForCommand(command string) []doctorCheck {
	out := make([]doctorCheck, 0, 2)
	add := func(level doctorLevel, subject string, message string) {
		out = append(out, doctorCheck{Level: level, Subject: subject, Message: message})
	}
	provider := inferProviderFromCommand(command)
	switch provider {
	case "":
		add(doctorWarn, "provider", "could not infer provider from summarizer command")
	case "claude":
		if commandExists("claude") {
			add(doctorOK, "provider:claude", "claude CLI found in PATH")
		} else {
			add(doctorFail, "provider:claude", "claude CLI not found in PATH")
		}
	case "codex":
		if commandExists("codex") {
			add(doctorOK, "provider:codex", "codex CLI found in PATH")
		} else {
			add(doctorFail, "provider:codex", "codex CLI not found in PATH")
		}
	case "cursor":
		if strings.TrimSpace(os.Getenv("CURSOR_API_KEY")) == "" {
			add(doctorFail, "provider:cursor", "CURSOR_API_KEY is not set")
		} else {
			add(doctorOK, "provider:cursor", "CURSOR_API_KEY is set")
		}
		if commandExists("curl") {
			add(doctorOK, "provider:cursor", "curl found in PATH")
		} else {
			add(doctorFail, "provider:cursor", "curl not found in PATH")
		}
		if commandExists("jq") {
			add(doctorOK, "provider:cursor", "jq found in PATH")
		} else {
			add(doctorFail, "provider:cursor", "jq not found in PATH")
		}
	}
	return out
}

func inferProviderFromCommand(command string) string {
	v := strings.ToLower(strings.TrimSpace(command))
	switch {
	case strings.Contains(v, "claude"):
		return "claude"
	case strings.Contains(v, "codex"):
		return "codex"
	case strings.Contains(v, "cursor"):
		return "cursor"
	default:
		return ""
	}
}

func looksPathLike(command string) bool {
	return strings.Contains(command, "/") || strings.HasPrefix(command, ".")
}

func checkExecutablePath(command string, projectRoot string) error {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return errors.New("empty command")
	}
	candidate := fields[0]
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(projectRoot, candidate)
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return fmt.Errorf("command path check failed: %v", err)
	}
	if info.IsDir() {
		return errors.New("command path points to a directory")
	}
	if info.Mode()&0o111 == 0 {
		return errors.New("command path is not executable")
	}
	return nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
