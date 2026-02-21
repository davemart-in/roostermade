package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	DirName              = ".recall"
	DBFileName           = "recall.db"
	ConfigFileName       = "config.json"
	DefaultSummaryThresh = 10
)

type Config struct {
	ProjectName        string   `json:"project_name"`
	SummaryThreshold   int      `json:"summary_threshold"`
	SummarizerProvider string   `json:"summarizer_provider,omitempty"`
	SummarizerCmd      string   `json:"summarizer_cmd,omitempty"`
	Docs               []string `json:"docs,omitempty"`
	Initialized        bool     `json:"initialized"`
}

func DirPath(projectRoot string) string {
	return filepath.Join(projectRoot, DirName)
}

func DBPath(projectRoot string) string {
	return filepath.Join(DirPath(projectRoot), DBFileName)
}

func ConfigPath(projectRoot string) string {
	return filepath.Join(DirPath(projectRoot), ConfigFileName)
}

func Default(projectRoot string) Config {
	return Config{
		ProjectName:      filepath.Base(projectRoot),
		SummaryThreshold: DefaultSummaryThresh,
	}
}

func Load(path string) (Config, error) {
	var cfg Config

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func Save(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	return os.WriteFile(path, data, 0o600)
}
