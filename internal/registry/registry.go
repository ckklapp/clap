package registry

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/rokiri/clap/internal/store"
)

type Kind string

const (
	KindArchPkg    Kind = "archpkg"
	KindAppImage   Kind = "appimage"
	KindBuild      Kind = "build"
	KindPacmanRepo Kind = "pacmanrepo"
)

type Entry struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Tag  string `json:"tag"`
	Kind Kind   `json:"kind"`
	Path string `json:"path"`
}

type db struct {
	Entries map[string]Entry `json:"entries"`
}

func path() string {
	return filepath.Join(store.Home(), "registry.json")
}

func load() db {
	d := db{Entries: map[string]Entry{}}
	data, err := os.ReadFile(path())
	if err != nil {
		return d
	}
	_ = json.Unmarshal(data, &d)
	if d.Entries == nil {
		d.Entries = map[string]Entry{}
	}
	return d
}

func save(d db) error {
	if err := os.MkdirAll(filepath.Dir(path()), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path(), data, 0o644)
}

func Record(e Entry) error {
	d := load()
	d.Entries[e.Name] = e
	return save(d)
}

func Forget(name string) error {
	d := load()
	delete(d.Entries, name)
	return save(d)
}

func All() map[string]Entry {
	return load().Entries
}

func Get(name string) (Entry, bool) {
	e, ok := load().Entries[name]
	return e, ok
}
