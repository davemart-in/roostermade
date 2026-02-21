package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/roostermade/recall/internal/bootstrap"
	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/docs"
	"github.com/roostermade/recall/internal/summarizer"
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

			selectedDocs, err := resolveSelectedDocs(reader, out)
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

	availableProviders := summarizer.DetectAvailableProviders()
	if len(availableProviders) > 0 {
		fmt.Fprintf(out, "detected summarizer providers: %s\n", strings.Join(availableProviders, ", "))
	} else {
		fmt.Fprintln(out, "detected summarizer providers: none")
	}
	provider, err := promptProvider(reader, out, summarizer.RecommendedProvider())
	if err != nil {
		return cfg, "", err
	}
	switch provider {
	case summarizer.ProviderNone:
		cfg.SummarizerProvider = ""
		cfg.SummarizerCmd = ""
		fmt.Fprintln(out, "summarizer setup skipped")
	default:
		wrapperPath, err := summarizer.WriteWrapper(projectRoot, provider)
		if err != nil {
			return cfg, "", err
		}
		cfg.SummarizerProvider = provider
		cfg.SummarizerCmd = wrapperPath
		fmt.Fprintf(out, "configured summarizer provider: %s\n", provider)
		fmt.Fprintf(out, "configured summarizer command: %s\n", wrapperPath)
		if !slices.Contains(availableProviders, provider) {
			fmt.Fprintf(
				errOut,
				"note: provider %s was not auto-detected; verify its prerequisites before summarizing\n",
				provider,
			)
		}
	}

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

	if strings.TrimSpace(os.Getenv("RECALL_SUMMARIZER_CMD")) == "" && strings.TrimSpace(cfg.SummarizerCmd) == "" {
		fmt.Fprintln(errOut, "note: no summarizer command configured; falling back to manual doc selection")
	}

	return cfg, contextText, nil
}

func resolveSelectedDocs(
	reader *bufio.Reader,
	out io.Writer,
) ([]string, error) {
	knownDocs := docs.KnownDocBases()

	selected := make([]string, 0, len(knownDocs))
	for _, base := range knownDocs {
		include, err := promptYesNo(
			reader,
			out,
			fmt.Sprintf("Include %s.md?", base),
			true,
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

func buildDocDraftInteractive(reader *bufio.Reader, out io.Writer, base string, projectName string, contextText string) (string, error) {
	title := docs.TitleFor(base)
	contextMap := parseContextSections(contextText)
	sections := docSectionsFor(base, contextMap)
	answers := make(map[string]string, len(sections))
	for _, section := range sections {
		label := section.Title
		if section.Prompt != "" {
			label = section.Prompt
		}
		answers[section.Title] = promptWithDefaultLine(reader, out, label, section.Default)
	}

	refinements := make([]string, 0)
	for {
		draft := renderDocDraft(title, projectName, contextText, sections, answers, refinements)
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

func renderDocDraft(
	title,
	projectName,
	contextText string,
	sections []docSection,
	answers map[string]string,
	refinements []string,
) string {
	var b strings.Builder
	b.WriteString("# " + title + "\n\n")
	b.WriteString("Project: " + strings.TrimSpace(projectName) + "\n\n")
	b.WriteString("## Context\n")
	b.WriteString(strings.TrimSpace(contextText) + "\n\n")
	for _, section := range sections {
		b.WriteString("## " + section.Title + "\n")
		val := strings.TrimSpace(answers[section.Title])
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

type docSection struct {
	Title   string
	Prompt  string
	Default string
}

func docSectionsFor(base string, contextMap map[string]string) []docSection {
	switch base {
	case "architecture":
		return []docSection{
			{
				Title:   "System Overview",
				Prompt:  "Architecture system overview",
				Default: combineNonEmpty(contextMap["Problem"], contextMap["Goals"]),
			},
			{Title: "Tech Stack", Prompt: "Architecture tech stack", Default: contextMap["Architecture Notes"]},
			{Title: "Data Model / DB Structure", Prompt: "Architecture data model / DB structure", Default: "TBD."},
			{Title: "File Structure", Prompt: "Architecture file structure", Default: "TBD."},
			{Title: "API Endpoints / Contracts", Prompt: "Architecture API endpoints / contracts", Default: "TBD."},
			{Title: "MCP Spec", Prompt: "Architecture MCP spec", Default: "TBD."},
			{Title: "Auth Spec", Prompt: "Architecture auth spec", Default: "TBD."},
			{Title: "Constraints", Prompt: "Architecture constraints", Default: combineNonEmpty(contextMap["Testing Expectations"], contextMap["Risks"])},
		}
	case "design":
		return []docSection{
			{Title: "Visual Direction", Prompt: "Design visual direction", Default: contextMap["Goals"]},
			{Title: "UX Principles", Prompt: "Design UX principles", Default: contextMap["Audience"]},
			{Title: "Interaction Patterns", Prompt: "Design interaction patterns", Default: contextMap["Problem"]},
			{Title: "Accessibility Expectations", Prompt: "Design accessibility expectations", Default: contextMap["Testing Expectations"]},
			{Title: "Responsive Behavior", Prompt: "Design responsive behavior", Default: "TBD."},
		}
	case "soul":
		return []docSection{
			{Title: "Principles", Prompt: "Soul principles", Default: contextMap["Goals"]},
			{Title: "Personality", Prompt: "Soul personality", Default: contextMap["Audience"]},
			{Title: "Non-Negotiables", Prompt: "Soul non-negotiables", Default: combineNonEmpty(contextMap["Architecture Notes"], contextMap["Risks"])},
			{Title: "Anti-Goals", Prompt: "Soul anti-goals", Default: contextMap["Problem"]},
		}
	default:
		return []docSection{
			{Title: "Objective", Prompt: "Objective", Default: contextMap["Goals"]},
			{Title: "In Scope", Prompt: "In Scope", Default: "TBD."},
			{Title: "Out of Scope", Prompt: "Out of Scope", Default: "TBD."},
			{Title: "Constraints and Risks", Prompt: "Constraints and Risks", Default: contextMap["Risks"]},
			{Title: "Acceptance Criteria", Prompt: "Acceptance Criteria", Default: contextMap["Testing Expectations"]},
			{Title: "Open Questions", Prompt: "Open Questions", Default: "TBD."},
		}
	}
}

func parseContextSections(contextText string) map[string]string {
	out := map[string]string{}
	current := ""
	var body strings.Builder

	flush := func() {
		if current == "" {
			return
		}
		value := strings.TrimSpace(body.String())
		if value == "" {
			value = "TBD."
		}
		out[current] = value
		body.Reset()
	}

	lines := strings.Split(contextText, "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "## ") {
			flush()
			current = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			continue
		}
		if current == "" {
			continue
		}
		if body.Len() > 0 {
			body.WriteString("\n")
		}
		body.WriteString(raw)
	}
	flush()
	return out
}

func combineNonEmpty(values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || strings.EqualFold(trimmed, "TBD.") {
			continue
		}
		parts = append(parts, trimmed)
	}
	if len(parts) == 0 {
		return "TBD."
	}
	return strings.Join(parts, "\n\n")
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

func promptProvider(reader *bufio.Reader, out io.Writer, def string) (string, error) {
	for {
		fmt.Fprintf(out, "Summarizer provider [%s] (claude/codex/cursor/none): ", def)
		raw, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		value := strings.TrimSpace(strings.ToLower(raw))
		if value == "" {
			value = def
		}
		if summarizer.IsValidProvider(value) {
			return value, nil
		}
		fmt.Fprintf(out, "invalid provider: %s\n", value)
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

func promptWithDefaultLine(reader *bufio.Reader, out io.Writer, label, def string) string {
	trimmedDefault := strings.TrimSpace(def)
	if trimmedDefault == "" {
		trimmedDefault = "TBD."
	}
	fmt.Fprintf(out, "%s [%s]: ", label, trimmedDefault)
	raw, err := reader.ReadString('\n')
	if err != nil {
		return trimmedDefault
	}
	value := strings.TrimSpace(raw)
	if value == "" {
		return trimmedDefault
	}
	return value
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
