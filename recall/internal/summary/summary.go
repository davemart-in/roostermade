package summary

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/roostermade/recall/internal/db"
)

const summarizerEnvVar = "RECALL_SUMMARIZER_CMD"
const summarizerTimeoutEnvVar = "RECALL_SUMMARIZER_TIMEOUT"
const defaultSummarizerTimeout = 90 * time.Second
const maxSummaryRepairAttempts = 1

var summaryBulletIDPattern = regexp.MustCompile(`(?m)^\s*[-*]\s+\[#([0-9]+)\]\s+.+$`)

func GenerateAndStore(store *db.Store) (db.Summary, bool, error) {
	return GenerateAndStoreWithCommand(store, "")
}

func GenerateAndStoreWithCommand(store *db.Store, configuredCmd string) (db.Summary, bool, error) {
	notes, err := store.ListUnsummarizedNotes()
	if err != nil {
		return db.Summary{}, false, err
	}
	if len(notes) == 0 {
		return db.Summary{}, false, nil
	}

	prompt := buildPrompt(notes)
	body, err := RunSummarizerCommandWith(configuredCmd, prompt)
	if err != nil {
		return db.Summary{}, false, err
	}
	expectedIDs := expectedNoteIDs(notes)
	if err := validateSummaryOutput(body, expectedIDs); err != nil {
		repaired, repairErr := retryRepairSummary(configuredCmd, notes, body, err)
		if repairErr != nil {
			return db.Summary{}, false, repairErr
		}
		body = repaired
	}

	highWaterID := notes[len(notes)-1].ID
	summary, err := store.CreateSummary(highWaterID, body)
	if err != nil {
		return db.Summary{}, false, err
	}

	return summary, true, nil
}

func RunSummarizerCommand(prompt string) (string, error) {
	return RunSummarizerCommandWith("", prompt)
}

func RunSummarizerCommandWith(configuredCmd string, prompt string) (string, error) {
	command := resolveSummarizerCommand(configuredCmd)
	if command == "" {
		return "", fmt.Errorf("%s is not set and config has no summarizer_cmd", summarizerEnvVar)
	}

	timeout, err := resolveSummarizerTimeout()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stdin = strings.NewReader(prompt)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("summarizer command timed out after %s", timeout)
		}
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			return "", fmt.Errorf("summarizer command failed: %w", err)
		}
		return "", fmt.Errorf("summarizer command failed: %w: %s", err, errText)
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return "", errors.New("summarizer command returned empty output")
	}

	return output, nil
}

func resolveSummarizerCommand(configuredCmd string) string {
	fromEnv := strings.TrimSpace(os.Getenv(summarizerEnvVar))
	if fromEnv != "" {
		return fromEnv
	}
	return strings.TrimSpace(configuredCmd)
}

func resolveSummarizerTimeout() (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(summarizerTimeoutEnvVar))
	if raw == "" {
		return defaultSummarizerTimeout, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration (example: 90s)", summarizerTimeoutEnvVar)
	}
	return d, nil
}

func buildPrompt(notes []db.Note) string {
	var b strings.Builder

	b.WriteString("Summarize these notes.\n")
	b.WriteString("Return exactly one short past-tense bullet per note.\n")
	b.WriteString("Each bullet must begin with the note id in this exact format: [#id].\n")
	b.WriteString("Output bullets only, no extra headings or commentary.\n\n")
	b.WriteString("Notes:\n")
	for _, note := range notes {
		b.WriteString(fmt.Sprintf("[#%d] %s\n", note.ID, note.Content))
	}

	return b.String()
}

func buildRepairPrompt(notes []db.Note, invalidOutput string, validationErr error) string {
	var b strings.Builder
	b.WriteString("Rewrite the summary into valid format.\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Return exactly one short past-tense bullet per note.\n")
	b.WriteString("- Each bullet must begin with [#id].\n")
	b.WriteString("- Use only the note IDs listed below, each exactly once.\n")
	b.WriteString("- Output bullets only.\n\n")

	b.WriteString("Expected note IDs:\n")
	for _, id := range expectedNoteIDs(notes) {
		b.WriteString(fmt.Sprintf("- %d\n", id))
	}
	b.WriteString("\n")

	b.WriteString("Validation failure:\n")
	b.WriteString(validationErr.Error())
	b.WriteString("\n\n")

	b.WriteString("Original invalid summary:\n")
	b.WriteString(invalidOutput)
	b.WriteString("\n")
	return b.String()
}

func retryRepairSummary(configuredCmd string, notes []db.Note, invalidOutput string, validationErr error) (string, error) {
	expectedIDs := expectedNoteIDs(notes)
	body := invalidOutput
	err := validationErr

	for i := 0; i < maxSummaryRepairAttempts; i++ {
		repairPrompt := buildRepairPrompt(notes, body, err)
		repaired, runErr := RunSummarizerCommandWith(configuredCmd, repairPrompt)
		if runErr != nil {
			return "", runErr
		}
		if validateErr := validateSummaryOutput(repaired, expectedIDs); validateErr == nil {
			return repaired, nil
		} else {
			body = repaired
			err = validateErr
		}
	}

	return "", fmt.Errorf("summary output invalid after repair attempt: %w", err)
}

func validateSummaryOutput(output string, expectedIDs []int64) error {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return errors.New("summary output is empty")
	}
	if len(expectedIDs) == 0 {
		return errors.New("expected IDs cannot be empty")
	}

	for _, raw := range strings.Split(trimmed, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if !summaryBulletIDPattern.MatchString(line) {
			return fmt.Errorf("summary contains non-bullet content: %q", line)
		}
	}

	foundIDs, err := extractBulletIDs(trimmed)
	if err != nil {
		return err
	}
	if len(foundIDs) != len(expectedIDs) {
		return fmt.Errorf("expected %d bullets, got %d", len(expectedIDs), len(foundIDs))
	}

	expectedCounts := make(map[int64]int, len(expectedIDs))
	for _, id := range expectedIDs {
		expectedCounts[id]++
	}
	foundCounts := make(map[int64]int, len(foundIDs))
	for _, id := range foundIDs {
		foundCounts[id]++
	}

	for id, expectedCount := range expectedCounts {
		gotCount := foundCounts[id]
		if gotCount < expectedCount {
			return fmt.Errorf("missing bullet for note id %d", id)
		}
		if gotCount > expectedCount {
			return fmt.Errorf("duplicate bullet for note id %d", id)
		}
	}

	extras := make([]int64, 0)
	for id := range foundCounts {
		if _, ok := expectedCounts[id]; !ok {
			extras = append(extras, id)
		}
	}
	if len(extras) > 0 {
		slices.Sort(extras)
		return fmt.Errorf("summary contains unexpected note ids: %v", extras)
	}

	return nil
}

func extractBulletIDs(output string) ([]int64, error) {
	matches := summaryBulletIDPattern.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return nil, errors.New("summary must contain bullets in format '- [#id] ...'")
	}

	ids := make([]int64, 0, len(matches))
	for _, match := range matches {
		var id int64
		if _, err := fmt.Sscanf(match[1], "%d", &id); err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid note id in bullet: %q", match[1])
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func expectedNoteIDs(notes []db.Note) []int64 {
	out := make([]int64, 0, len(notes))
	for _, note := range notes {
		out = append(out, note.ID)
	}
	return out
}
