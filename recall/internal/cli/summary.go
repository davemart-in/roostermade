package cli

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/roostermade/recall/internal/summary"
	"github.com/spf13/cobra"
)

const summaryPreviewChars = 120

func newSummaryCmd() *cobra.Command {
	summaryCmd := &cobra.Command{
		Use:   "summary",
		Short: "Manage summaries",
	}

	summaryCmd.AddCommand(
		newSummaryAddCmd(),
		newSummaryListCmd(),
		newSummaryGetCmd(),
	)

	return summaryCmd
}

func newSummaryAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add",
		Short: "Manually trigger summarization of unsummarized thoughts",
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

			createdSummary, didSummarize, err := summary.GenerateAndStore(store)
			if err != nil {
				return err
			}
			if !didSummarize {
				cmd.Println("no unsummarized thoughts")
				return nil
			}

			cmd.Printf("created summary #%d\n", createdSummary.ID)
			cmd.Printf("through thought #%d\n", createdSummary.ThoughtID)
			cmd.Printf("created_at: %s\n", createdSummary.CreatedAt.Format(time.RFC3339))
			return nil
		},
	}
}

func newSummaryListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List summaries",
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

			summaries, err := store.ListSummaries(100, 0)
			if err != nil {
				return err
			}
			if len(summaries) == 0 {
				cmd.Println("no summaries found")
				return nil
			}

			for _, item := range summaries {
				preview := summarizePreview(item.Body, summaryPreviewChars)
				cmd.Printf(
					"id:%d | thought_id:%d | created_at:%s | %s\n",
					item.ID,
					item.ThoughtID,
					item.CreatedAt.Format(time.RFC3339),
					preview,
				)
			}

			return nil
		},
	}
}

func newSummaryGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a summary by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil || id <= 0 {
				return fmt.Errorf("invalid summary id: %q", args[0])
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

			item, err := store.GetSummary(id)
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("summary %d not found", id)
			}
			if err != nil {
				return err
			}

			cmd.Printf("id: %d\n", item.ID)
			cmd.Printf("thought_id: %d\n", item.ThoughtID)
			cmd.Printf("created_at: %s\n", item.CreatedAt.Format(time.RFC3339))
			cmd.Printf("body:\n%s\n", item.Body)

			return nil
		},
	}
}

func summarizePreview(body string, maxChars int) string {
	oneLine := strings.Join(strings.Fields(body), " ")
	if len(oneLine) <= maxChars {
		return oneLine
	}

	return oneLine[:maxChars] + "..."
}
