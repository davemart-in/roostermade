package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/db"
	"github.com/roostermade/recall/internal/docs"
	"github.com/roostermade/recall/internal/snapshot"
	"github.com/spf13/cobra"
)

const defaultContextMaxChars = snapshot.DefaultMaxChars
const defaultContextSummaryLimit = snapshot.DefaultSummaryLimit

type contextPart struct {
	filename string
	text     string
}

func newContextCmd() *cobra.Command {
	var full bool
	var maxChars int
	var summaryLimit int
	var summaryFull bool
	var includeDocIndex bool
	var query string
	var queryNoteLimit int
	var querySummaryLimit int

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

			parts, err := buildContextParts(
				cwd,
				cfg,
				summaryLimit,
				summaryFull,
				includeDocIndex,
				query,
				queryNoteLimit,
				querySummaryLimit,
			)
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
	cmd.Flags().IntVar(&summaryLimit, "summary-limit", defaultContextSummaryLimit, "Number of recent summaries to include (0 to disable)")
	cmd.Flags().BoolVar(&summaryFull, "summary-full", false, "Include full summary body content in recent summaries section")
	cmd.Flags().BoolVar(&includeDocIndex, "include-doc-index", true, "Include a docs index section")
	cmd.Flags().StringVar(&query, "query", "", "Include matching notes/summaries for this query")
	cmd.Flags().IntVar(&queryNoteLimit, "query-note-limit", snapshot.DefaultQueryLimit, "Number of matching notes to include when using --query")
	cmd.Flags().IntVar(&querySummaryLimit, "query-summary-limit", snapshot.DefaultQueryLimit, "Number of matching summaries to include when using --query")
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

func buildContextParts(
	projectRoot string,
	cfg config.Config,
	summaryLimit int,
	summaryFull bool,
	includeDocIndex bool,
	query string,
	queryNoteLimit int,
	querySummaryLimit int,
) ([]contextPart, error) {
	conn, err := db.Open(config.DBPath(projectRoot))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	parts, err := snapshot.BuildParts(projectRoot, cfg, db.NewStore(conn), snapshot.Options{
		SummaryLimit:      summaryLimit,
		SummaryFull:       summaryFull,
		IncludeDocIndex:   includeDocIndex,
		Query:             query,
		QueryNoteLimit:    queryNoteLimit,
		QuerySummaryLimit: querySummaryLimit,
	})
	if err != nil {
		return nil, err
	}

	out := make([]contextPart, 0, len(parts))
	for _, part := range parts {
		out = append(out, contextPart{filename: part.Filename, text: part.Text})
	}
	return out, nil
}

func assembleContextOutput(parts []contextPart, full bool, maxChars int) (string, bool, error) {
	shared := make([]snapshot.Part, 0, len(parts))
	for _, part := range parts {
		shared = append(shared, snapshot.Part{Filename: part.filename, Text: part.text})
	}
	return snapshot.AssembleOutput(shared, full, maxChars)
}

func firstDocDescriptionLine(body string) string {
	return snapshot.FirstDocDescriptionLine(body)
}

func runeCount(s string) int {
	return snapshot.RuneCount(s)
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
