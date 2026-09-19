package aur

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/rokiri/clap/internal/store"
)

type DBEntry struct {
	Name        string `json:"name"`
	PackageBase string `json:"package_base"`
	Version     string `json:"version"`
	AsDep       bool   `json:"as_dep"`
}

type DB struct {
	Entries map[string]DBEntry `json:"entries"`
}

func dbPath() string {
	return filepath.Join(store.Home(), "aur", "installed.json")
}

func loadDB() DB {
	d := DB{Entries: map[string]DBEntry{}}
	data, err := os.ReadFile(dbPath())
	if err != nil {
		return d
	}
	_ = json.Unmarshal(data, &d)
	if d.Entries == nil {
		d.Entries = map[string]DBEntry{}
	}
	return d
}

func saveDB(d DB) error {
	if err := os.MkdirAll(filepath.Dir(dbPath()), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dbPath(), data, 0o644)
}

func recordInstall(e DBEntry) error {
	d := loadDB()
	d.Entries[e.Name] = e
	return saveDB(d)
}

func forgetInstall(name string) error {
	d := loadDB()
	delete(d.Entries, name)
	return saveDB(d)
}

func Tracked() map[string]DBEntry {
	return loadDB().Entries
}
