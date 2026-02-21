package cli

import (
	"archive/zip"
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/roostermade/recall/internal/bootstrap"
	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/transfer"
	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "Export recall data to a zip archive",
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

			if err := transfer.EnsureExportDocsExist(cwd, cfg); err != nil {
				return err
			}

			noteCount, err := store.CountNotes()
			if err != nil {
				return err
			}
			summaryCount, err := store.CountSummaries()
			if err != nil {
				return err
			}

			now := time.Now()
			manifest := transfer.BuildManifest(cfg, noteCount, summaryCount, now)
			outputPath, err := transfer.ResolveExportPath(cwd, now)
			if err != nil {
				return err
			}

			if err := transfer.Export(cwd, cfg, manifest, outputPath); err != nil {
				return err
			}

			cmd.Printf("export created: %s\n", filepath.Base(outputPath))
			return nil
		},
	}
}

func newImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import <zipfile>",
		Short: "Import recall data from an export zip",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			zipPath := strings.TrimSpace(args[0])
			if zipPath == "" {
				return errors.New("zipfile path cannot be empty")
			}

			zr, err := zip.OpenReader(zipPath)
			if err != nil {
				return err
			}
			defer zr.Close()

			manifest, _, err := transfer.ReadManifestFromZip(zr)
			if err != nil {
				return err
			}
			entries, err := transfer.FindRequiredImportEntries(zr, manifest)
			if err != nil {
				return err
			}

			recallDir := config.DirPath(cwd)
			if _, err := os.Stat(recallDir); err == nil {
				ok, err := confirmOverwrite(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.OutOrStderr())
				if err != nil {
					return err
				}
				if !ok {
					cmd.Println("import cancelled")
					return nil
				}
				if err := os.RemoveAll(recallDir); err != nil {
					return err
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}

			if err := transfer.WriteImportedRecall(cwd, entries, manifest); err != nil {
				return err
			}

			// Ensure .gitignore contains .recall/recall.db after import.
			if _, err := bootstrap.EnsureBaseArtifacts(cwd); err != nil {
				return err
			}

			cmd.Printf("import complete: %s\n", filepath.Base(zipPath))
			cmd.Printf("project: %s\n", manifest.ProjectName)
			cmd.Printf("docs imported: %d\n", len(manifest.DocList))
			return nil
		},
	}
}

func confirmOverwrite(in io.Reader, out io.Writer, errOut io.Writer) (bool, error) {
	file, ok := in.(*os.File)
	if !ok {
		return false, errors.New("cannot prompt for overwrite in non-interactive mode; rerun interactively")
	}
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	if (info.Mode() & os.ModeCharDevice) == 0 {
		return false, errors.New("cannot prompt for overwrite in non-interactive mode; rerun interactively")
	}

	reader := bufio.NewReader(in)
	fmt.Fprint(out, "Overwrite existing .recall/? [y/N]: ")
	raw, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	answer := strings.TrimSpace(strings.ToLower(raw))
	switch answer {
	case "y", "yes":
		return true, nil
	case "", "n", "no":
		return false, nil
	default:
		fmt.Fprintln(errOut, "invalid response; defaulting to no")
		return false, nil
	}
}
