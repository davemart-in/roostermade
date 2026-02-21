package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/roostermade/recall/internal/bootstrap"
	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/docs"
	"github.com/roostermade/recall/internal/summary"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize or update Recall for this project",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			if _, err := bootstrap.EnsureBaseArtifacts(cwd); err != nil {
				return err
			}

			cfg, err := config.Load(config.ConfigPath(cwd))
			if err != nil {
				return err
			}

			reader := bufio.NewReader(cmd.InOrStdin())
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()

			cfg, contextText, err := runInitWizard(reader, out, errOut, cwd, cfg)
			if err != nil {
				return err
			}

			selectedDocs, err := resolveSelectedDocs(reader, out, errOut, contextText)
			if err != nil {
				return err
			}

			createdCount := 0
			skippedCount := 0
			for _, base := range selectedDocs {
				filename := base + ".md"
				docs.RegisterDoc(&cfg, filename)

				path := docs.DocPath(cwd, filename)
				exists, nonEmpty, err := fileExistsAndNonEmpty(path)
				if err != nil {
					return err
				}
				if exists && nonEmpty {
					skippedCount++
					fmt.Fprintf(out, "skipping existing doc: .recall/%s\n", filename)
					continue
				}

				draft, err := buildDocDraftInteractive(reader, out, base, cfg.ProjectName, contextText)
				if err != nil {
					return err
				}
				if err := os.WriteFile(path, []byte(draft), 0o644); err != nil {
					return err
				}
				createdCount++
				fmt.Fprintf(out, "wrote .recall/%s\n", filename)
			}

			cfg.Initialized = true
			if err := config.Save(config.ConfigPath(cwd), cfg); err != nil {
				return err
			}

			fmt.Fprintf(out, "init complete\n")
			fmt.Fprintf(out, "docs created: %d\n", createdCount)
			fmt.Fprintf(out, "docs skipped (already existed): %d\n", skippedCount)
			return nil
		},
	}
}

func runInitWizard(reader *bufio.Reader, out io.Writer, errOut io.Writer, projectRoot string, cfg config.Config) (config.Config, string, error) {
	projectDefault := cfg.ProjectName
	if strings.TrimSpace(projectDefault) == "" {
		projectDefault = config.Default(projectRoot).ProjectName
	}

	projectName, err := promptWithDefault(reader, out, "Project name", projectDefault)
	if err != nil {
		return cfg, "", err
	}
	cfg.ProjectName = projectName

	thresholdDefault := cfg.SummaryThreshold
	if thresholdDefault <= 0 {
		thresholdDefault = config.DefaultSummaryThresh
	}
	summaryThreshold, err := promptIntWithDefault(reader, out, "Summary threshold", thresholdDefault)
	if err != nil {
		return cfg, "", err
	}
	cfg.SummaryThreshold = summaryThreshold

	contextDraft := buildContextFromQuestions(reader, out)
	contextPath := docs.DocPath(projectRoot, docs.ContextFilename)
	contextText := contextDraft
	if exists, _, err := fileExistsAndNonEmpty(contextPath); err != nil {
		return cfg, "", err
	} else if exists {
		data, err := os.ReadFile(contextPath)
		if err != nil {
			return cfg, "", err
		}
		contextText = string(data)
		fmt.Fprintln(out, "keeping existing .recall/context.md")
	} else {
		if err := os.WriteFile(contextPath, []byte(contextDraft), 0o644); err != nil {
			return cfg, "", err
		}
		fmt.Fprintln(out, "created .recall/context.md")
	}

	docs.RegisterDoc(&cfg, docs.ContextFilename)

	if strings.TrimSpace(os.Getenv("RECALL_SUMMARIZER_CMD")) == "" {
		fmt.Fprintln(errOut, "note: RECALL_SUMMARIZER_CMD not set; falling back to manual doc selection")
	}

	return cfg, contextText, nil
}

func resolveSelectedDocs(reader *bufio.Reader, out io.Writer, errOut io.Writer, contextText string) ([]string, error) {
	knownDocs := docs.KnownDocBases()
	recommended := []string{}

	if strings.TrimSpace(os.Getenv("RECALL_SUMMARIZER_CMD")) != "" {
		recs, err := recommendDocs(contextText, knownDocs)
		if err != nil {
			fmt.Fprintf(errOut, "warning: failed to recommend docs via LLM: %v\n", err)
		} else {
			recommended = recs
			if len(recommended) > 0 {
				fmt.Fprintf(out, "recommended docs: %s\n", strings.Join(recommended, ", "))
			}
		}
	}

	recommendedSet := make(map[string]bool, len(recommended))
	for _, base := range recommended {
		recommendedSet[base] = true
	}

	selected := make([]string, 0, len(knownDocs))
	for _, base := range knownDocs {
		include, err := promptYesNo(
			reader,
			out,
			fmt.Sprintf("Include %s.md?", base),
			recommendedSet[base],
		)
		if err != nil {
			return nil, err
		}
		if include {
			selected = append(selected, base)
		}
	}

	return selected, nil
}

func recommendDocs(contextText string, knownDocs []string) ([]string, error) {
	prompt := "Based on this project context, recommend helpful docs from this allowed list only:\n" +
		strings.Join(knownDocs, ", ") + "\n\n" +
		"Return only a comma-separated list of slugs from the allowed list.\n\n" +
		"Context:\n" + contextText

	raw, err := summary.RunSummarizerCommand(prompt)
	if err != nil {
		return nil, err
	}

	knownSet := make(map[string]bool, len(knownDocs))
	for _, d := range knownDocs {
		knownSet[d] = true
	}

	tokenRe := regexp.MustCompile(`[a-z][a-z-]+`)
	tokens := tokenRe.FindAllString(strings.ToLower(raw), -1)
	tokenSet := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		if knownSet[token] {
			tokenSet[token] = true
		}
	}

	out := make([]string, 0, len(knownDocs))
	for _, d := range knownDocs {
		if tokenSet[d] {
			out = append(out, d)
		}
	}
	return out, nil
}

func buildDocDraftInteractive(reader *bufio.Reader, out io.Writer, base string, projectName string, contextText string) (string, error) {
	title := docs.TitleFor(base)
	answers := map[string]string{
		"Objective":             promptLine(reader, out, fmt.Sprintf("%s objective", title)),
		"In Scope":              promptLine(reader, out, fmt.Sprintf("%s in scope", title)),
		"Out of Scope":          promptLine(reader, out, fmt.Sprintf("%s out of scope", title)),
		"Constraints and Risks": promptLine(reader, out, fmt.Sprintf("%s constraints/risks", title)),
		"Acceptance Criteria":   promptLine(reader, out, fmt.Sprintf("%s acceptance criteria", title)),
		"Open Questions":        promptLine(reader, out, fmt.Sprintf("%s open questions", title)),
	}

	refinements := make([]string, 0)
	for {
		draft := renderDocDraft(title, projectName, contextText, answers, refinements)
		fmt.Fprintf(out, "\n--- Draft Preview: %s ---\n%s\n--- End Draft ---\n", base+".md", draft)

		ok, err := promptYesNo(reader, out, "Is this satisfactory?", true)
		if err != nil {
			return "", err
		}
		if ok {
			return draft, nil
		}

		refinement := promptLine(reader, out, "What should be improved")
		if strings.TrimSpace(refinement) != "" {
			refinements = append(refinements, refinement)
		}
	}
}

func renderDocDraft(title, projectName, contextText string, answers map[string]string, refinements []string) string {
	var b strings.Builder
	b.WriteString("# " + title + "\n\n")
	b.WriteString("Project: " + strings.TrimSpace(projectName) + "\n\n")
	b.WriteString("## Context\n")
	b.WriteString(strings.TrimSpace(contextText) + "\n\n")
	for _, section := range []string{
		"Objective",
		"In Scope",
		"Out of Scope",
		"Constraints and Risks",
		"Acceptance Criteria",
		"Open Questions",
	} {
		b.WriteString("## " + section + "\n")
		val := strings.TrimSpace(answers[section])
		if val == "" {
			val = "TBD."
		}
		b.WriteString(val + "\n\n")
	}
	if len(refinements) > 0 {
		b.WriteString("## Refinement Notes\n")
		for _, note := range refinements {
			b.WriteString("- " + strings.TrimSpace(note) + "\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func buildContextFromQuestions(reader *bufio.Reader, out io.Writer) string {
	questions := []struct {
		Heading string
		Prompt  string
	}{
		{Heading: "Problem", Prompt: "What core problem is this project solving?"},
		{Heading: "Audience", Prompt: "Who is the primary audience/user?"},
		{Heading: "Goals", Prompt: "What are the near-term goals?"},
		{Heading: "Architecture Notes", Prompt: "Any architecture/tech constraints to capture?"},
		{Heading: "Testing Expectations", Prompt: "Any testing/reliability expectations?"},
		{Heading: "Risks", Prompt: "What are key risks or unknowns?"},
	}

	var b strings.Builder
	b.WriteString("# Project Context\n\n")
	for _, q := range questions {
		answer := promptLine(reader, out, q.Prompt)
		b.WriteString("## " + q.Heading + "\n")
		if strings.TrimSpace(answer) == "" {
			b.WriteString("TBD.\n\n")
		} else {
			b.WriteString(strings.TrimSpace(answer) + "\n\n")
		}
	}
	return b.String()
}

func promptWithDefault(reader *bufio.Reader, out io.Writer, label string, def string) (string, error) {
	for {
		fmt.Fprintf(out, "%s [%s]: ", label, def)
		raw, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		value := strings.TrimSpace(raw)
		if value == "" {
			return def, nil
		}
		return value, nil
	}
}

func promptIntWithDefault(reader *bufio.Reader, out io.Writer, label string, def int) (int, error) {
	for {
		fmt.Fprintf(out, "%s [%d]: ", label, def)
		raw, err := reader.ReadString('\n')
		if err != nil {
			return 0, err
		}
		value := strings.TrimSpace(raw)
		if value == "" {
			return def, nil
		}
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			fmt.Fprintln(out, "please enter a positive integer")
			continue
		}
		return n, nil
	}
}

func promptYesNo(reader *bufio.Reader, out io.Writer, label string, defaultYes bool) (bool, error) {
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}
	for {
		fmt.Fprintf(out, "%s %s: ", label, suffix)
		raw, err := reader.ReadString('\n')
		if err != nil {
			return false, err
		}
		value := strings.TrimSpace(strings.ToLower(raw))
		if value == "" {
			return defaultYes, nil
		}
		switch value {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(out, "please answer yes or no")
		}
	}
}

func promptLine(reader *bufio.Reader, out io.Writer, label string) string {
	fmt.Fprintf(out, "%s: ", label)
	raw, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimSpace(raw)
}

func fileExistsAndNonEmpty(path string) (bool, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, false, nil
		}
		return false, false, err
	}
	return true, info.Size() > 0, nil
}
