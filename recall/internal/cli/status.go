package cli

import (
	"os"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show project memory status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			store, cfg, closeDB, err := openStore(cwd)
			if err != nil {
				return err
			}
			defer closeDB()

			thoughtCount, err := store.CountThoughts()
			if err != nil {
				return err
			}
			summaryCount, err := store.CountSummaries()
			if err != nil {
				return err
			}
			unsummarizedCount, err := store.CountUnsummarizedThoughts()
			if err != nil {
				return err
			}

			cmd.Printf("thoughts: %d\n", thoughtCount)
			cmd.Printf("summaries: %d\n", summaryCount)
			cmd.Printf("unsummarized_thoughts: %d\n", unsummarizedCount)
			cmd.Printf("docs: %d\n", len(cfg.Docs))
			return nil
		},
	}
}
