package cli

import "github.com/spf13/cobra"

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "recall",
		Short:         "Project-scoped memory for AI agents",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newThoughtCmd())

	return cmd
}
