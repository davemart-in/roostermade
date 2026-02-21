package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/roostermade/recall/internal/config"
	"github.com/roostermade/recall/internal/db"
)

const dbGitignorePattern = ".recall/recall.db"
const recallDirGitignorePattern = "recall.db"

var ErrNotInitialized = errors.New("recall is not initialized")

func EnsureBaseArtifacts(projectRoot string) (bool, error) {
	recallDir := config.DirPath(projectRoot)

	createdDir, err := ensureRecallDir(recallDir)
	if err != nil {
		return false, err
	}

	if err := ensureConfig(projectRoot); err != nil {
		return false, err
	}

	if err := ensureDB(projectRoot); err != nil {
		return false, err
	}

	if err := ensureDBGitignore(projectRoot); err != nil {
		return false, err
	}
	if err := ensureRecallDirGitignore(projectRoot); err != nil {
		return false, err
	}

	return createdDir, nil
}

// EnsureProjectInitialized is retained for compatibility with previous phases.
func EnsureProjectInitialized(projectRoot string) (bool, error) {
	return EnsureBaseArtifacts(projectRoot)
}

func IsInitialized(projectRoot string) (bool, error) {
	configPath := config.ConfigPath(projectRoot)
	if _, err := os.Stat(configPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return false, err
	}

	return cfg.Initialized, nil
}

func RequireInitialized(projectRoot string) error {
	initialized, err := IsInitialized(projectRoot)
	if err != nil {
		return err
	}
	if !initialized {
		return ErrNotInitialized
	}
	return nil
}

func ensureRecallDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return false, fmt.Errorf("%s exists but is not a directory", path)
		}
		return false, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}

	if err := os.MkdirAll(path, 0o755); err != nil {
		return false, err
	}

	return true, nil
}

func ensureConfig(projectRoot string) error {
	configPath := config.ConfigPath(projectRoot)
	if _, err := os.Stat(configPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return config.Save(configPath, config.Default(projectRoot))
}

func ensureDB(projectRoot string) error {
	conn, err := db.Open(config.DBPath(projectRoot))
	if err != nil {
		return err
	}
	defer conn.Close()

	return nil
}

func ensureDBGitignore(projectRoot string) error {
	path := filepath.Join(projectRoot, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if strings.Contains(string(data), dbGitignorePattern) {
		return nil
	}

	line := dbGitignorePattern + "\n"
	if len(data) == 0 {
		return os.WriteFile(path, []byte(line), 0o644)
	}

	if !strings.HasSuffix(string(data), "\n") {
		data = append(data, '\n')
	}
	data = append(data, []byte(line)...)

	return os.WriteFile(path, data, 0o644)
}

func ensureRecallDirGitignore(projectRoot string) error {
	path := filepath.Join(config.DirPath(projectRoot), ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if strings.Contains(string(data), recallDirGitignorePattern) {
		return nil
	}

	line := recallDirGitignorePattern + "\n"
	if len(data) == 0 {
		return os.WriteFile(path, []byte(line), 0o644)
	}

	if !strings.HasSuffix(string(data), "\n") {
		data = append(data, '\n')
	}
	data = append(data, []byte(line)...)

	return os.WriteFile(path, data, 0o644)
}
