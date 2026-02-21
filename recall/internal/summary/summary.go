package summary

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/roostermade/recall/internal/db"
)

const summarizerEnvVar = "RECALL_SUMMARIZER_CMD"

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

	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = strings.NewReader(prompt)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
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
