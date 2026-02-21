package cli

import (
	"errors"
	"os"

	"github.com/roostermade/recall/internal/bootstrap"
	recallmcp "github.com/roostermade/recall/internal/mcp"
	"github.com/spf13/cobra"
)

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run Recall MCP server over stdio",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = cmd
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			if err := bootstrap.RequireInitialized(cwd); err != nil {
				if errors.Is(err, bootstrap.ErrNotInitialized) {
					return notInitializedError()
				}
				return err
			}

			return recallmcp.RunStdio(cwd)
		},
	}
}
