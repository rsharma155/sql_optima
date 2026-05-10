package config

import (
	"os"
	"path/filepath"
)

const timescalePersistFile = "timescale_connection.enc.json"

// TimescalePersistPath returns the path to the encrypted Timescale connection file.
func TimescalePersistPath(configPath string) string {
	base := filepath.Clean(filepath.Join(filepath.Dir(configPath), "data"))
	return filepath.Join(base, timescalePersistFile)
}

// TimescalePersistFileExists reports whether the encrypted Timescale connection file is present.
func TimescalePersistFileExists(configPath string) bool {
	st, err := os.Stat(TimescalePersistPath(configPath))
	if err != nil {
		return false
	}
	return !st.IsDir() && st.Size() > 0
}
