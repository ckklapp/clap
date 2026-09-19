package archpkg

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rokiri/clap/internal/ghrelease"
)

func pacmanArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	case "386":
		return "i686"
	case "arm":
		return "armv7h"
	default:
		return runtime.GOARCH
	}
}

func IsPackageAsset(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".pkg.tar.zst") || strings.HasSuffix(lower, ".pkg.tar.xz")
}

func FindAsset(rel *ghrelease.Release) *ghrelease.Asset {
	arch := pacmanArch()
	var anyMatch *ghrelease.Asset
	for i := range rel.Assets {
		a := &rel.Assets[i]
		if !IsPackageAsset(a.Name) {
			continue
		}
		lower := strings.ToLower(a.Name)
		if strings.Contains(lower, "-"+arch+".") || strings.Contains(lower, "-"+arch+"-") {
			return a
		}
		if strings.Contains(lower, "-any.") {
			anyMatch = a
		}
	}
	return anyMatch
}

func Install(downloadPath string) error {
	fmt.Printf("clap: sudo pacman -U %s\n", filepath.Base(downloadPath))
	cmd := exec.Command("sudo", "pacman", "-U", downloadPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pacman -U failed: %w", err)
	}
	return nil
}
