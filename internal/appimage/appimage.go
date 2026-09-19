package appimage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rokiri/clap/internal/ghrelease"
	"github.com/rokiri/clap/internal/store"
)

func IsAppImageAsset(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".appimage")
}

func archAliases() []string {
	switch runtime.GOARCH {
	case "amd64":
		return []string{"amd64", "x86_64", "x64"}
	case "arm64":
		return []string{"arm64", "aarch64"}
	case "386":
		return []string{"386", "i386", "i686"}
	default:
		return []string{runtime.GOARCH}
	}
}

func FindAsset(rel *ghrelease.Release) *ghrelease.Asset {
	var fallback *ghrelease.Asset
	aliases := archAliases()
	for i := range rel.Assets {
		a := &rel.Assets[i]
		if !IsAppImageAsset(a.Name) {
			continue
		}
		lower := strings.ToLower(a.Name)
		for _, ar := range aliases {
			if strings.Contains(lower, ar) {
				return a
			}
		}
		if fallback == nil {
			fallback = a
		}
	}
	return fallback
}

func Install(name, downloadPath string) (string, error) {
	if err := os.MkdirAll(store.AppImageDir(), 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(store.AppImageDir(), name+".AppImage")
	if err := os.Rename(downloadPath, dest); err != nil {
		if err := copyFile(downloadPath, dest); err != nil {
			return "", fmt.Errorf("placing AppImage: %w", err)
		}
	}
	if err := os.Chmod(dest, 0o755); err != nil {
		return "", err
	}

	if err := os.MkdirAll(store.LocalBinDir(), 0o755); err != nil {
		return "", err
	}
	link := filepath.Join(store.LocalBinDir(), name)
	_ = os.Remove(link)
	if err := os.Symlink(dest, link); err != nil {
		return "", fmt.Errorf("symlinking %s -> %s: %w", link, dest, err)
	}

	fmt.Printf("clap: installed AppImage %s -> %s (linked as %s)\n", name, dest, link)
	fmt.Println("clap: note — no .desktop entry / icon integration yet, this is CLI-launchable only")
	return dest, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
