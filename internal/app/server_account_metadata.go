package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Independent extension; native auth, config and runtime formats stay upstream.
type serverAccountMetadata struct {
	LastSuccess        *time.Time `json:"last_success_at,omitempty"`
	ImportCompleteness string     `json:"import_completeness,omitempty"`
}

var accountMetadataMu sync.Mutex

func readServerAccountMetadata(directory string) serverAccountMetadata {
	var value serverAccountMetadata
	raw, err := os.ReadFile(filepath.Join(directory, "server-account.json"))
	if err == nil {
		_ = json.Unmarshal(raw, &value)
	}
	return value
}

func updateServerAccountMetadata(directory string, update func(*serverAccountMetadata)) error {
	accountMetadataMu.Lock()
	defer accountMetadataMu.Unlock()
	value := readServerAccountMetadata(directory)
	update(&value)
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".server-account-*.tmp")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(raw); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(path, filepath.Join(directory, "server-account.json"))
}
