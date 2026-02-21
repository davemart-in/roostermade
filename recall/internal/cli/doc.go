package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/roostermade/recall/internal/bootstrap"
	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/docs"
	"github.com/spf13/cobra"
)

func newDocCmd() *cobra.Command {
	docCmd := &cobra.Command{
		Use:   "doc",
		Short: "Manage docs",
	}

	docCmd.AddCommand(
		newDocAddCmd(),
		newDocEditCmd(),
		newDocListCmd(),
	)

	return docCmd
}

func newDocAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name>",
		Short: "Create and register a doc",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			cfg, err := ensureProjectAndLoadConfig(cwd)
			if err != nil {
				return err
			}

			filename, base, err := docs.NormalizeDocName(args[0])
			if err != nil {
				return err
			}

			template := ""
			if docs.IsKnown(base) {
				template = docs.TemplateFor(base)
			}

			created, err := docs.EnsureDocFile(cwd, filename, template)
			if err != nil {
				return err
			}

			added := docs.RegisterDoc(&cfg, filename)
			if added {
				if err := config.Save(config.ConfigPath(cwd), cfg); err != nil {
					return err
				}
			}

			if created {
				cmd.Printf("created .recall/%s\n", filename)
			} else {
				cmd.Printf("doc already exists: .recall/%s\n", filename)
			}

			if added {
				cmd.Printf("registered %s\n", filename)
			} else {
				cmd.Printf("already registered: %s\n", filename)
			}

			return nil
		},
	}
}

func newDocEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: "Open a doc in $EDITOR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			cfg, err := ensureProjectAndLoadConfig(cwd)
			if err != nil {
				return err
			}

			filename, base, err := docs.NormalizeDocName(args[0])
			if err != nil {
				return err
			}

			template := ""
			if docs.IsKnown(base) {
				template = docs.TemplateFor(base)
			}

			if _, err := docs.EnsureDocFile(cwd, filename, template); err != nil {
				return err
			}

			added := docs.RegisterDoc(&cfg, filename)
			if added {
				if err := config.Save(config.ConfigPath(cwd), cfg); err != nil {
					return err
				}
			}

			docPath := docs.DocPath(cwd, filename)
			if err := openEditor(docPath); err != nil {
				return err
			}

			return nil
		},
	}
}

func newDocListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered docs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			cfg, err := ensureProjectAndLoadConfig(cwd)
			if err != nil {
				return err
			}

			entries, err := docs.ListRegistered(cwd, cfg)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				cmd.Println("no docs registered")
				return nil
			}

			for _, entry := range entries {
				if entry.Missing {
					cmd.Printf("%s | modified:missing\n", entry.Filename)
					continue
				}
				cmd.Printf("%s | modified:%s\n", entry.Filename, entry.ModifiedAt.Format(time.RFC3339))
			}

			return nil
		},
	}
}

func ensureProjectAndLoadConfig(projectRoot string) (config.Config, error) {
	if err := bootstrap.RequireInitialized(projectRoot); err != nil {
		if errors.Is(err, bootstrap.ErrNotInitialized) {
			return config.Config{}, notInitializedError()
		}
		return config.Config{}, err
	}

	return config.Load(config.ConfigPath(projectRoot))
}

func openEditor(path string) error {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = "vi"
	}

	cmd := exec.Command("sh", "-c", editor+` "$1"`, "recall-doc-edit", path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("open editor: %w", err)
	}
	return nil
}
