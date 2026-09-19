package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rokiri/clap/internal/installer"
	"github.com/rokiri/clap/internal/store"
)

const manifestFile = "clap.json"
const lockFile = "clap.lock.json"

type Dep struct {
	URL string `json:"url"`
	Tag string `json:"tag,omitempty"`
}

type Manifest struct {
	Apps map[string]Dep `json:"apps"`
	Libs map[string]Dep `json:"libs"`
}

type LockEntry struct {
	URL  string `json:"url"`
	Tag  string `json:"tag"`
	Kind string `json:"kind"`
}

type Lock struct {
	Entries map[string]LockEntry `json:"entries"`
}

func Init(dir string) error {
	p := filepath.Join(dir, manifestFile)
	if _, err := os.Stat(p); err == nil {
		return fmt.Errorf("%s already exists", p)
	}
	m := Manifest{Apps: map[string]Dep{}, Libs: map[string]Dep{}}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("clap: created %s\n", p)
	return nil
}

func load(dir string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		return m, fmt.Errorf("no %s found in %s (run `clap init` first): %w", manifestFile, dir, err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	if m.Apps == nil {
		m.Apps = map[string]Dep{}
	}
	if m.Libs == nil {
		m.Libs = map[string]Dep{}
	}
	return m, nil
}

func loadLock(dir string) Lock {
	l := Lock{Entries: map[string]LockEntry{}}
	data, err := os.ReadFile(filepath.Join(dir, lockFile))
	if err != nil {
		return l
	}
	_ = json.Unmarshal(data, &l)
	if l.Entries == nil {
		l.Entries = map[string]LockEntry{}
	}
	return l
}

func saveLock(dir string, l Lock) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, lockFile), data, 0o644)
}

func Sync(dir string) error {
	m, err := load(dir)
	if err != nil {
		return err
	}
	lock := loadLock(dir)

	install := func(name string, dep Dep, isLib bool) error {
		existing, ok := lock.Entries[name]
		if ok && existing.URL == dep.URL && existing.Tag == dep.Tag && store.IsInstalled(name) {
			fmt.Printf("clap: %s already in sync (%s)\n", name, dep.URL)
			return nil
		}
		res, err := installer.Install(installer.Options{
			URL: dep.URL, Name: name, Tag: dep.Tag, Lib: isLib,
		})
		if err != nil {
			return fmt.Errorf("syncing %s: %w", name, err)
		}
		lock.Entries[name] = LockEntry{URL: dep.URL, Tag: res.Tag, Kind: res.Kind}
		return nil
	}

	for name, dep := range m.Apps {
		if err := install(name, dep, false); err != nil {
			return err
		}
	}
	for name, dep := range m.Libs {
		if err := install(name, dep, true); err != nil {
			return err
		}
	}

	if err := saveLock(dir, lock); err != nil {
		return err
	}
	fmt.Printf("clap: synced %d app(s), %d lib(s) -> %s\n", len(m.Apps), len(m.Libs), filepath.Join(dir, lockFile))
	return nil
}
