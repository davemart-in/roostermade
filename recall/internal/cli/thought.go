package cli

import (
	"database/sql"
	"errors"
	"fmt"
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

func newThoughtCmd() *cobra.Command {
	thoughtCmd := &cobra.Command{
		Use:   "thought",
		Short: "Manage thoughts",
	}

	thoughtCmd.AddCommand(
		newThoughtAddCmd(),
		newThoughtListCmd(),
		newThoughtGetCmd(),
	)

	return thoughtCmd
}

func newThoughtAddCmd() *cobra.Command {
	var llm string
	var model string

	cmd := &cobra.Command{
		Use:   `add "<content>"`,
		Short: "Add a thought",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			content := strings.TrimSpace(args[0])
			if content == "" {
				return errors.New("thought content cannot be empty")
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

			thought, err := store.CreateThought(content, toOptionalString(llm), toOptionalString(model))
			if err != nil {
				return err
			}

			cmd.Printf("added thought #%d\n", thought.ID)
			cmd.Printf("created_at: %s\n", thought.CreatedAt.Format(time.RFC3339))
			cmd.Printf("content: %s\n", thought.Content)
			if thought.LLM.Valid {
				cmd.Printf("llm: %s\n", thought.LLM.String)
			}
			if thought.Model.Valid {
				cmd.Printf("model: %s\n", thought.Model.String)
			}

			unsummarizedCount, err := store.CountUnsummarizedThoughts()
			if err != nil {
				return err
			}
			if unsummarizedCount > cfg.SummaryThreshold {
				createdSummary, didSummarize, err := summary.GenerateAndStore(store)
				if err != nil {
					cmd.PrintErrf("warning: auto-summary failed: %v\n", err)
				} else if didSummarize {
					cmd.Printf(
						"auto-summary created #%d (through thought #%d)\n",
						createdSummary.ID,
						createdSummary.ThoughtID,
					)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&llm, "llm", "", "LLM/provider used for this thought")
	cmd.Flags().StringVar(&model, "model", "", "Model used for this thought")

	return cmd
}

func newThoughtListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List thoughts",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			store, _, closeDB, err := openStore(cwd)
			if err != nil {
				return err
			}
			defer closeDB()

			thoughts, err := store.ListThoughts(100, 0)
			if err != nil {
				return err
			}

			if len(thoughts) == 0 {
				cmd.Println("no thoughts found")
				return nil
			}

			for _, thought := range thoughts {
				parts := []string{
					fmt.Sprintf("[#%d]", thought.ID),
					thought.CreatedAt.Format(time.RFC3339),
					thought.Content,
				}
				if thought.LLM.Valid {
					parts = append(parts, "llm="+thought.LLM.String)
				}
				if thought.Model.Valid {
					parts = append(parts, "model="+thought.Model.String)
				}
				cmd.Println(strings.Join(parts, " | "))
			}

			return nil
		},
	}
}

func newThoughtGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a thought by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil || id <= 0 {
				return fmt.Errorf("invalid thought id: %q", args[0])
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

			thought, err := store.GetThought(id)
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("thought %d not found", id)
			}
			if err != nil {
				return err
			}

			cmd.Printf("id: %d\n", thought.ID)
			cmd.Printf("created_at: %s\n", thought.CreatedAt.Format(time.RFC3339))
			if thought.LLM.Valid {
				cmd.Printf("llm: %s\n", thought.LLM.String)
			}
			if thought.Model.Valid {
				cmd.Printf("model: %s\n", thought.Model.String)
			}
			cmd.Printf("content: %s\n", thought.Content)

			return nil
		},
	}
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
