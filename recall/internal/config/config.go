package config

import "path/filepath"

const (
	DirName    = ".recall"
	DBFileName = "recall.db"
)

func DirPath(projectRoot string) string {
	return filepath.Join(projectRoot, DirName)
}

func DBPath(projectRoot string) string {
	return filepath.Join(DirPath(projectRoot), DBFileName)
}
