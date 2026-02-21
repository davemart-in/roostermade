package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/docs"
	"github.com/spf13/cobra"
)

const defaultContextMaxChars = 16000

type contextPart struct {
	filename string
	text     string
}

func newContextCmd() *cobra.Command {
	var full bool
	var maxChars int

	cmd := &cobra.Command{
		Use:   "context",
		Short: "Print project context",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			cfg, err := ensureProjectAndLoadConfig(cwd)
			if err != nil {
				return err
			}

			cfg, err = ensureContextDoc(cmd, cwd, cfg)
			if err != nil {
				return err
			}

			parts, err := buildContextParts(cwd)
			if err != nil {
				return err
			}
			if len(parts) == 0 {
				return errors.New("no context docs available")
			}

			output, truncated, err := assembleContextOutput(parts, full, maxChars)
			if err != nil {
				return err
			}
			cmd.Print(output)
			if truncated {
				cmd.Printf(
					"\n--- context truncated at %d chars; rerun with --full for complete output ---\n",
					maxChars,
				)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&full, "full", false, "Disable max-size safeguard and print full context")
	cmd.Flags().IntVar(&maxChars, "max-chars", defaultContextMaxChars, "Maximum chars to print when not using --full")
	return cmd
}

func ensureContextDoc(cmd *cobra.Command, projectRoot string, cfg config.Config) (config.Config, error) {
	contextPath := docs.DocPath(projectRoot, docs.ContextFilename)
	if _, err := os.Stat(contextPath); err == nil {
		return cfg, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return cfg, err
	}

	if !isInteractiveInput(cmd.InOrStdin()) {
		return cfg, fmt.Errorf(
			"%s not found; run `recall init` to generate it",
			filepath.ToSlash(filepath.Join(".recall", docs.ContextFilename)),
		)
	}

	reader := bufioReaderFrom(cmd.InOrStdin())
	recoverNow, err := promptYesNo(
		reader,
		cmd.OutOrStdout(),
		fmt.Sprintf(
			"%s is missing. Recreate it now via guided prompts?",
			filepath.ToSlash(filepath.Join(".recall", docs.ContextFilename)),
		),
		true,
	)
	if err != nil {
		return cfg, err
	}
	if !recoverNow {
		return cfg, fmt.Errorf(
			"%s not found; run `recall init` to generate it",
			filepath.ToSlash(filepath.Join(".recall", docs.ContextFilename)),
		)
	}

	contextText := buildContextFromQuestions(reader, cmd.OutOrStdout())
	if err := os.WriteFile(contextPath, []byte(contextText), 0o644); err != nil {
		return cfg, err
	}
	if docs.RegisterDoc(&cfg, docs.ContextFilename) {
		if err := config.Save(config.ConfigPath(projectRoot), cfg); err != nil {
			return cfg, err
		}
	}
	fmt.Fprintf(cmd.OutOrStdout(), "recreated .recall/%s\n", docs.ContextFilename)
	return cfg, nil
}

func buildContextParts(projectRoot string) ([]contextPart, error) {
	data, err := os.ReadFile(docs.DocPath(projectRoot, docs.ContextFilename))
	if err != nil {
		return nil, err
	}
	return []contextPart{
		{
			filename: docs.ContextFilename,
			text:     renderContextSection(docs.ContextFilename, string(data)),
		},
	}, nil
}

func renderContextSection(filename, body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		trimmed = "TBD."
	}
	return fmt.Sprintf("=== .recall/%s ===\n%s\n\n", filename, trimmed)
}

func assembleContextOutput(parts []contextPart, full bool, maxChars int) (string, bool, error) {
	if full {
		var b strings.Builder
		for _, part := range parts {
			b.WriteString(part.text)
		}
		return b.String(), false, nil
	}
	if maxChars <= 0 {
		return "", false, errors.New("--max-chars must be greater than 0")
	}

	var b strings.Builder
	truncated := false
	remaining := maxChars

	for _, part := range parts {
		if remaining <= 0 {
			truncated = true
			break
		}
		partLen := runeCount(part.text)
		if partLen <= remaining {
			b.WriteString(part.text)
			remaining -= partLen
			continue
		}

		b.WriteString(truncateByRunes(part.text, remaining))
		truncated = true
		break
	}

	return b.String(), truncated, nil
}

func truncateByRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}

func runeCount(s string) int {
	return len([]rune(s))
}

func isInteractiveInput(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func bufioReaderFrom(in io.Reader) *bufio.Reader {
	if reader, ok := in.(*bufio.Reader); ok {
		return reader
	}
	return bufio.NewReader(in)
}
