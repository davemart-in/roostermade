package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/roostermade/recall/internal/docs"
	"github.com/spf13/cobra"
)

func newContextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "context",
		Short: "Print project context document",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			if _, err := ensureProjectAndLoadConfig(cwd); err != nil {
				return err
			}

			contextPath := docs.DocPath(cwd, docs.ContextFilename)
			data, err := os.ReadFile(contextPath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf(
						"%s not found; rerun `recall init` to generate it",
						filepath.ToSlash(filepath.Join(".recall", docs.ContextFilename)),
					)
				}
				return err
			}

			cmd.Print(string(data))
			return nil
		},
	}
}
