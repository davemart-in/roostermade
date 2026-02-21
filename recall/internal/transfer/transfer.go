package transfer

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/docs"
)

const (
	manifestFilename = "recall-manifest.json"
	dbFilename       = "recall.db"
)

type Manifest struct {
	ProjectName      string   `json:"project_name"`
	ExportDate       string   `json:"export_date"`
	NoteCount        int      `json:"note_count"`
	SummaryCount     int      `json:"summary_count"`
	SummaryThreshold int      `json:"summary_threshold"`
	SummarizerCmd    string   `json:"summarizer_cmd,omitempty"`
	DocList          []string `json:"doc_list"`
}

func EnsureExportDocsExist(projectRoot string, cfg config.Config) error {
	missing := make([]string, 0)
	for _, filename := range cfg.Docs {
		path := docs.DocPath(projectRoot, filename)
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				missing = append(missing, filename)
				continue
			}
			return err
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing registered docs: %s", strings.Join(missing, ", "))
	}
	return nil
}

func BuildManifest(cfg config.Config, noteCount, summaryCount int, now time.Time) Manifest {
	docList := slices.Clone(cfg.Docs)
	return Manifest{
		ProjectName:      cfg.ProjectName,
		ExportDate:       now.UTC().Format(time.RFC3339),
		NoteCount:        noteCount,
		SummaryCount:     summaryCount,
		SummaryThreshold: cfg.SummaryThreshold,
		SummarizerCmd:    cfg.SummarizerCmd,
		DocList:          docList,
	}
}

func ResolveExportPath(projectRoot string, now time.Time) (string, error) {
	date := now.Format("2006-01-02")
	baseDir := filepath.Join(config.DirPath(projectRoot), "exports")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", err
	}
	base := filepath.Join(baseDir, fmt.Sprintf("recall-export-%s.zip", date))

	if _, err := os.Stat(base); errors.Is(err, os.ErrNotExist) {
		return base, nil
	} else if err != nil {
		return "", err
	}

	for i := 2; ; i++ {
		candidate := filepath.Join(baseDir, fmt.Sprintf("recall-export-%s-%d.zip", date, i))
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
}

func Export(projectRoot string, cfg config.Config, manifest Manifest, outputZip string) error {
	if err := EnsureExportDocsExist(projectRoot, cfg); err != nil {
		return err
	}

	out, err := os.Create(outputZip)
	if err != nil {
		return err
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()

	if err := addFileToZip(zw, config.DBPath(projectRoot), dbFilename); err != nil {
		return err
	}
	for _, docFilename := range cfg.Docs {
		if err := addFileToZip(zw, docs.DocPath(projectRoot, docFilename), docFilename); err != nil {
			return err
		}
	}

	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	manifestJSON = append(manifestJSON, '\n')
	w, err := zw.Create(manifestFilename)
	if err != nil {
		return err
	}
	if _, err := w.Write(manifestJSON); err != nil {
		return err
	}

	return nil
}

func addFileToZip(zw *zip.Writer, sourcePath, zipName string) error {
	f, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer f.Close()

	w, err := zw.Create(zipName)
	if err != nil {
		return err
	}

	_, err = io.Copy(w, f)
	return err
}

func ParseAndValidateManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest: %w", err)
	}

	if strings.TrimSpace(manifest.ProjectName) == "" {
		return Manifest{}, errors.New("manifest missing project_name")
	}
	if strings.TrimSpace(manifest.ExportDate) == "" {
		return Manifest{}, errors.New("manifest missing export_date")
	}
	if _, err := time.Parse(time.RFC3339, manifest.ExportDate); err != nil {
		return Manifest{}, fmt.Errorf("manifest export_date must be RFC3339: %w", err)
	}
	if manifest.NoteCount < 0 {
		return Manifest{}, errors.New("manifest note_count must be >= 0")
	}
	if manifest.SummaryCount < 0 {
		return Manifest{}, errors.New("manifest summary_count must be >= 0")
	}
	if manifest.SummaryThreshold <= 0 {
		return Manifest{}, errors.New("manifest summary_threshold must be > 0")
	}
	if manifest.DocList == nil {
		return Manifest{}, errors.New("manifest missing doc_list")
	}
	for _, docName := range manifest.DocList {
		if strings.TrimSpace(docName) == "" {
			return Manifest{}, errors.New("manifest contains empty doc name")
		}
		if strings.Contains(docName, "/") || strings.Contains(docName, "\\") || strings.Contains(docName, "..") {
			return Manifest{}, fmt.Errorf("manifest contains invalid doc path: %s", docName)
		}
	}

	return manifest, nil
}

func FindRequiredImportEntries(zr *zip.ReadCloser, manifest Manifest) (map[string]*zip.File, error) {
	entries := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		if err := ValidateZipPath(f.Name); err != nil {
			return nil, err
		}
		entries[f.Name] = f
	}

	if _, ok := entries[manifestFilename]; !ok {
		return nil, errors.New("zip missing recall-manifest.json")
	}
	if _, ok := entries[dbFilename]; !ok {
		return nil, errors.New("zip missing recall.db")
	}
	for _, docName := range manifest.DocList {
		if _, ok := entries[docName]; !ok {
			return nil, fmt.Errorf("zip missing doc listed in manifest: %s", docName)
		}
	}
	return entries, nil
}

func ValidateZipPath(name string) error {
	if filepath.IsAbs(name) {
		return fmt.Errorf("unsafe absolute zip path: %s", name)
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == "" {
		return fmt.Errorf("unsafe empty zip path: %s", name)
	}
	if strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return fmt.Errorf("unsafe zip path traversal: %s", name)
	}
	if strings.Contains(clean, "/") || strings.Contains(clean, "\\") {
		return fmt.Errorf("unsupported nested zip path: %s", name)
	}
	return nil
}

func WriteImportedRecall(projectRoot string, entries map[string]*zip.File, manifest Manifest) error {
	recallDir := config.DirPath(projectRoot)
	if err := os.MkdirAll(recallDir, 0o755); err != nil {
		return err
	}

	if err := extractZipEntry(entries[dbFilename], config.DBPath(projectRoot), 0o644); err != nil {
		return err
	}
	for _, docName := range manifest.DocList {
		if err := extractZipEntry(entries[docName], docs.DocPath(projectRoot, docName), 0o644); err != nil {
			return err
		}
	}

	cfg := config.Default(projectRoot)
	cfg.ProjectName = manifest.ProjectName
	cfg.SummaryThreshold = manifest.SummaryThreshold
	cfg.SummarizerCmd = strings.TrimSpace(manifest.SummarizerCmd)
	cfg.Docs = slices.Clone(manifest.DocList)
	cfg.Initialized = true

	return config.Save(config.ConfigPath(projectRoot), cfg)
}

func extractZipEntry(zf *zip.File, destPath string, mode os.FileMode) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}

func ReadManifestFromZip(zr *zip.ReadCloser) (Manifest, []byte, error) {
	for _, f := range zr.File {
		if f.Name != manifestFilename {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return Manifest{}, nil, err
		}
		defer rc.Close()

		data, err := io.ReadAll(rc)
		if err != nil {
			return Manifest{}, nil, err
		}

		manifest, err := ParseAndValidateManifest(data)
		if err != nil {
			return Manifest{}, nil, err
		}

		return manifest, data, nil
	}
	return Manifest{}, nil, errors.New("zip missing recall-manifest.json")
}
