package store

import (
	"os"
	"path/filepath"
)

func Home() string {
	if h := os.Getenv("CLAP_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".clap")
}

func AppsDir() string { return filepath.Join(Home(), "apps") }
func RunDir() string  { return filepath.Join(Home(), "run") }
func TmpDir() string  { return filepath.Join(Home(), "tmp") }

func CacheDir() string {
	if c := os.Getenv("XDG_CACHE_HOME"); c != "" {
		return filepath.Join(c, "clap")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".cache", "clap")
}

func BuildDir(name string) string {
	return filepath.Join(CacheDir(), "build", name)
}

func LocalBinDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".local", "bin")
}

func AppImageDir() string {
	return filepath.Join(Home(), "appimages")
}

func Resolve(name string) (path string, ext string) {
	p := filepath.Join(AppsDir(), name+".clap")
	if _, err := os.Stat(p); err == nil {
		return p, ".clap"
	}
	p = filepath.Join(AppsDir(), name+".clapl")
	if _, err := os.Stat(p); err == nil {
		return p, ".clapl"
	}
	return "", ""
}

func IsInstalled(name string) bool {
	p, _ := Resolve(name)
	return p != ""
}
