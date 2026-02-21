package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/docs"
	"github.com/spf13/cobra"
)

type configMenuItem struct {
	Label  string
	Action func() error
}

func newConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "View and edit configuration interactively",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			cfg, err := ensureProjectAndLoadConfig(cwd)
			if err != nil {
				return err
			}

			reader := bufio.NewReader(cmd.InOrStdin())
			out := cmd.OutOrStdout()

			saveConfig := func() error {
				return config.Save(config.ConfigPath(cwd), cfg)
			}

			for {
				items := buildConfigMenuItems(cwd, &cfg, reader, out, saveConfig)
				cmd.Println("Select an item to edit:")
				for i, item := range items {
					cmd.Printf("%d) %s\n", i+1, item.Label)
				}
				cmd.Println("0) quit")

				cmd.Print("> ")
				raw, err := reader.ReadString('\n')
				if err != nil {
					return err
				}

				choice := strings.TrimSpace(strings.ToLower(raw))
				if choice == "0" || choice == "q" || choice == "quit" {
					return nil
				}

				index, err := strconv.Atoi(choice)
				if err != nil || index < 1 || index > len(items) {
					cmd.Println("invalid selection")
					continue
				}

				if err := items[index-1].Action(); err != nil {
					return err
				}
			}
		},
	}
}

func buildConfigMenuItems(
	projectRoot string,
	cfg *config.Config,
	reader *bufio.Reader,
	out io.Writer,
	saveConfig func() error,
) []configMenuItem {
	items := []configMenuItem{
		{
			Label: fmt.Sprintf("project_name: %s", cfg.ProjectName),
			Action: func() error {
				value, err := promptWithDefault(reader, out, "Project name", cfg.ProjectName)
				if err != nil {
					return err
				}
				value = strings.TrimSpace(value)
				if value == "" {
					return fmt.Errorf("project name cannot be empty")
				}
				if value == cfg.ProjectName {
					return nil
				}
				cfg.ProjectName = value
				return saveConfig()
			},
		},
		{
			Label: fmt.Sprintf("summary_threshold: %d", cfg.SummaryThreshold),
			Action: func() error {
				value, err := promptIntWithDefault(reader, out, "Summary threshold", cfg.SummaryThreshold)
				if err != nil {
					return err
				}
				if value == cfg.SummaryThreshold {
					return nil
				}
				cfg.SummaryThreshold = value
				return saveConfig()
			},
		},
	}

	for _, filename := range cfg.Docs {
		docFilename := filename
		docPath := docs.DocPath(projectRoot, docFilename)
		docLabel := fmt.Sprintf("doc: %s", docFilename)
		if _, err := os.Stat(docPath); err != nil {
			if os.IsNotExist(err) {
				docLabel += " (missing)"
			}
		}
		items = append(items, configMenuItem{
			Label: docLabel,
			Action: func() error {
				docPath := docs.DocPath(projectRoot, docFilename)
				if _, err := os.Stat(docPath); err != nil {
					if os.IsNotExist(err) {
						createNow, err := promptYesNo(
							reader,
							out,
							fmt.Sprintf(".recall/%s is missing. Create it now?", docFilename),
							true,
						)
						if err != nil {
							return err
						}
						if !createNow {
							return nil
						}
						if _, err := docs.EnsureDocFile(projectRoot, docFilename, ""); err != nil {
							return err
						}
					} else {
						return err
					}
				}
				return openEditor(docPath)
			},
		})
	}

	return items
}
