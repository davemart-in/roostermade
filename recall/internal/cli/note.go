package cli

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/roostermade/recall/internal/bootstrap"
	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/db"
	"github.com/roostermade/recall/internal/summary"
	"github.com/spf13/cobra"
)

func newNoteCmd() *cobra.Command {
	noteCmd := &cobra.Command{
		Use:   "note",
		Short: "Manage notes",
	}

	noteCmd.AddCommand(
		newNoteAddCmd(),
		newNoteListCmd(),
		newNoteGetCmd(),
		newNoteDeleteCmd(),
		newNoteSearchCmd(),
	)

	return noteCmd
}

func newNoteAddCmd() *cobra.Command {
	var llm string
	var model string

	cmd := &cobra.Command{
		Use:   `add [<content>]`,
		Short: "Add a note",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			content := ""
			if len(args) == 1 {
				content = strings.TrimSpace(args[0])
			} else {
				if isInteractiveInput(cmd.InOrStdin()) {
					return errors.New("note content cannot be empty (provide an argument or pipe stdin)")
				}
				data, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return err
				}
				content = strings.TrimSpace(string(data))
			}
			if content == "" {
				return errors.New("note content cannot be empty")
			}

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			store, cfg, closeDB, err := openStore(cwd)
			if err != nil {
				return err
			}
			defer closeDB()

			note, err := store.CreateNote(content, toOptionalString(llm), toOptionalString(model))
			if err != nil {
				return err
			}

			cmd.Printf("added note #%d\n", note.ID)
			cmd.Printf("created_at: %s\n", note.CreatedAt.Format(time.RFC3339))
			cmd.Printf("content: %s\n", note.Content)
			if note.LLM.Valid {
				cmd.Printf("llm: %s\n", note.LLM.String)
			}
			if note.Model.Valid {
				cmd.Printf("model: %s\n", note.Model.String)
			}

			unsummarizedCount, err := store.CountUnsummarizedNotes()
			if err != nil {
				return err
			}
			if unsummarizedCount > cfg.SummaryThreshold {
				createdSummary, didSummarize, err := summary.GenerateAndStoreWithCommand(store, cfg.SummarizerCmd)
				if err != nil {
					cmd.PrintErrf("warning: auto-summary failed: %v\n", err)
				} else if didSummarize {
					cmd.Printf(
						"auto-summary created #%d (through note #%d)\n",
						createdSummary.ID,
						createdSummary.NoteID,
					)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&llm, "llm", "", "LLM/provider used for this note")
	cmd.Flags().StringVar(&model, "model", "", "Model used for this note")

	return cmd
}

func newNoteDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a note by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil || id <= 0 {
				return fmt.Errorf("invalid note id: %q", args[0])
			}

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			store, _, closeDB, err := openStore(cwd)
			if err != nil {
				return err
			}
			defer closeDB()

			if err := store.DeleteNote(id); errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("note %d not found", id)
			} else if err != nil {
				return err
			}

			cmd.Printf("deleted note #%d\n", id)
			return nil
		},
	}
}

func newNoteListCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List notes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if limit <= 0 {
				return errors.New("limit must be greater than 0")
			}

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			store, _, closeDB, err := openStore(cwd)
			if err != nil {
				return err
			}
			defer closeDB()

			notes, err := store.ListNotes(limit, 0)
			if err != nil {
				return err
			}

			if len(notes) == 0 {
				cmd.Println("no notes found")
				return nil
			}

			for _, note := range notes {
				parts := []string{
					fmt.Sprintf("[#%d]", note.ID),
					note.CreatedAt.Format(time.RFC3339),
					note.Content,
				}
				if note.LLM.Valid {
					parts = append(parts, "llm="+note.LLM.String)
				}
				if note.Model.Valid {
					parts = append(parts, "model="+note.Model.String)
				}
				cmd.Println(strings.Join(parts, " | "))
			}

			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum number of notes to return")
	return cmd
}

func newNoteGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a note by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil || id <= 0 {
				return fmt.Errorf("invalid note id: %q", args[0])
			}

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			store, _, closeDB, err := openStore(cwd)
			if err != nil {
				return err
			}
			defer closeDB()

			note, err := store.GetNote(id)
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("note %d not found", id)
			}
			if err != nil {
				return err
			}

			cmd.Printf("id: %d\n", note.ID)
			cmd.Printf("created_at: %s\n", note.CreatedAt.Format(time.RFC3339))
			if note.LLM.Valid {
				cmd.Printf("llm: %s\n", note.LLM.String)
			}
			if note.Model.Valid {
				cmd.Printf("model: %s\n", note.Model.String)
			}
			cmd.Printf("content: %s\n", note.Content)

			return nil
		},
	}
}

func newNoteSearchCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search notes by content",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit <= 0 {
				return errors.New("limit must be greater than 0")
			}

			query := strings.TrimSpace(args[0])
			if query == "" {
				return errors.New("query cannot be empty")
			}

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			store, _, closeDB, err := openStore(cwd)
			if err != nil {
				return err
			}
			defer closeDB()

			notes, err := store.SearchNotes(query, limit, 0)
			if err != nil {
				return err
			}
			if len(notes) == 0 {
				cmd.Println("no matching notes found")
				return nil
			}

			for _, note := range notes {
				parts := []string{
					fmt.Sprintf("[#%d]", note.ID),
					note.CreatedAt.Format(time.RFC3339),
					note.Content,
				}
				if note.LLM.Valid {
					parts = append(parts, "llm="+note.LLM.String)
				}
				if note.Model.Valid {
					parts = append(parts, "model="+note.Model.String)
				}
				cmd.Println(strings.Join(parts, " | "))
			}

			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum number of notes to return")
	return cmd
}

func openStore(projectRoot string) (*db.Store, config.Config, func() error, error) {
	if err := bootstrap.RequireInitialized(projectRoot); err != nil {
		if errors.Is(err, bootstrap.ErrNotInitialized) {
			return nil, config.Config{}, nil, notInitializedError()
		}
		return nil, config.Config{}, nil, err
	}

	cfg, err := config.Load(config.ConfigPath(projectRoot))
	if err != nil {
		return nil, config.Config{}, nil, err
	}

	conn, err := db.Open(config.DBPath(projectRoot))
	if err != nil {
		return nil, config.Config{}, nil, err
	}

	return db.NewStore(conn), cfg, conn.Close, nil
}

func notInitializedError() error {
	return errors.New("Recall is not initialized in this project. Run `recall init` first.")
}

func toOptionalString(raw string) *string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
