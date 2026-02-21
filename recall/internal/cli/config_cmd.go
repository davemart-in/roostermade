package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/docs"
	"github.com/roostermade/recall/internal/summarizer"
	"github.com/spf13/cobra"
)

type configMenuItem struct {
	Label  string
	Action func() error
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "View and edit configuration interactively",
		RunE:  runConfigInteractive,
	}
	cmd.AddCommand(
		newConfigGetCmd(),
		newConfigSetCmd(),
	)
	return cmd
}

func runConfigInteractive(cmd *cobra.Command, _ []string) error {
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
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a config value",
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
			value, err := getConfigValue(cfg, args[0])
			if err != nil {
				return err
			}
			cmd.Println(value)
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a writable config value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			cfg, err := ensureProjectAndLoadConfig(cwd)
			if err != nil {
				return err
			}
			if err := setConfigValue(&cfg, args[0], args[1]); err != nil {
				return err
			}
			if err := config.Save(config.ConfigPath(cwd), cfg); err != nil {
				return err
			}
			cmd.Printf("updated %s\n", normalizeConfigKey(args[0]))
			return nil
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

func getConfigValue(cfg config.Config, key string) (string, error) {
	switch normalizeConfigKey(key) {
	case "project_name":
		return cfg.ProjectName, nil
	case "summary_threshold":
		return strconv.Itoa(cfg.SummaryThreshold), nil
	case "summarizer_provider":
		return cfg.SummarizerProvider, nil
	case "summarizer_cmd":
		return cfg.SummarizerCmd, nil
	case "docs":
		raw, err := json.Marshal(cfg.Docs)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	case "initialized":
		if cfg.Initialized {
			return "true", nil
		}
		return "false", nil
	default:
		return "", fmt.Errorf("unknown key %q", key)
	}
}

func setConfigValue(cfg *config.Config, key string, value string) error {
	switch normalizeConfigKey(key) {
	case "project_name":
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return errors.New("project_name cannot be empty")
		}
		cfg.ProjectName = trimmed
		return nil
	case "summary_threshold":
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n <= 0 {
			return errors.New("summary_threshold must be a positive integer")
		}
		cfg.SummaryThreshold = n
		return nil
	case "summarizer_provider":
		provider := strings.ToLower(strings.TrimSpace(value))
		if !summarizer.IsValidProvider(provider) {
			return errors.New("summarizer_provider must be one of: claude, codex, cursor, none")
		}
		if provider == summarizer.ProviderNone {
			cfg.SummarizerProvider = ""
			return nil
		}
		cfg.SummarizerProvider = provider
		return nil
	case "summarizer_cmd":
		cfg.SummarizerCmd = strings.TrimSpace(value)
		return nil
	case "docs", "initialized":
		return fmt.Errorf("%s is read-only", normalizeConfigKey(key))
	default:
		return fmt.Errorf("unknown key %q", key)
	}
}

func normalizeConfigKey(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}
