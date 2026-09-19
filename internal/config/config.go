package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/rokiri/clap/internal/store"
)

type Config struct {
	DefaultProvider string `json:"default_provider,omitempty"`
}

func path() string {
	return filepath.Join(store.Home(), "config.json")
}

func Load() Config {
	var c Config
	data, err := os.ReadFile(path())
	if err != nil {
		return c
	}
	_ = json.Unmarshal(data, &c)
	return c
}

func Save(c Config) error {
	if err := os.MkdirAll(filepath.Dir(path()), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path(), data, 0o644)
}

func SetDefaultProvider(name string) error {
	c := Load()
	c.DefaultProvider = name
	return Save(c)
}

func DefaultProvider() (string, bool) {
	c := Load()
	return c.DefaultProvider, c.DefaultProvider != ""
}
