package selfupgrade

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rokiri/clap/internal/archivex"
	"github.com/rokiri/clap/internal/ghrelease"
	"github.com/rokiri/clap/internal/manifest"
	"github.com/rokiri/clap/internal/store"
)

const selfOwner = "ckklapp"
const selfRepo = "clappm"

func Upgrade() error {
	fmt.Printf("clap: checking %s/%s for a newer clap...\n", selfOwner, selfRepo)
	rel, err := ghrelease.Fetch(selfOwner, selfRepo, "")
	if err != nil {
		return fmt.Errorf("could not check for updates: %w", err)
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	current := strings.TrimPrefix(manifest.Version, "v")
	if latest == current {
		fmt.Printf("clap: already up to date (%s)\n", manifest.Version)
		return nil
	}
	fmt.Printf("clap: upgrading clap %s -> %s\n", manifest.Version, rel.TagName)

	asset := ghrelease.FindBinaryAsset(rel, runtime.GOOS, runtime.GOARCH)
	if asset == nil {
		return fmt.Errorf("no release asset for %s/%s found in %s/%s %s", runtime.GOOS, runtime.GOARCH, selfOwner, selfRepo, rel.TagName)
	}
	fmt.Printf("clap: found asset %q\n", asset.Name)

	if err := os.MkdirAll(store.TmpDir(), 0o755); err != nil {
		return err
	}
	dl := filepath.Join(store.TmpDir(), "clap-self-"+asset.Name)
	if err := ghrelease.Download(asset.BrowserDownloadURL, dl); err != nil {
		return fmt.Errorf("downloading asset: %w", err)
	}

	if cs := ghrelease.FindChecksumAsset(rel); cs != nil {
		ok, err := ghrelease.VerifyChecksum(cs.BrowserDownloadURL, asset.Name, dl)
		switch {
		case err != nil:
			return fmt.Errorf("checksum verification failed: %w", err)
		case ok:
			fmt.Println("clap: checksum verified OK")
		default:
			fmt.Println("clap: no checksum entry found for this asset, skipping verification")
		}
	}

	newBin := dl
	lower := strings.ToLower(asset.Name)
	if strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		fmt.Println("clap: unpacking archive...")
		extracted := filepath.Join(store.TmpDir(), "clap-self-extracted")
		if err := archivex.ExtractBinary(dl, extracted, "clap"); err != nil {
			return fmt.Errorf("extracting archive: %w", err)
		}
		newBin = extracted
	}
	if err := os.Chmod(newBin, 0o755); err != nil {
		return err
	}

	if err := replaceRunningBinary(newBin); err != nil {
		return err
	}

	fmt.Printf("clap: upgraded to %s\n", rel.TagName)
	return nil
}

var executableFunc = os.Executable

func replaceRunningBinary(newBin string) error {
	currentPath, err := executableFunc()
	if err != nil {
		return fmt.Errorf("locating the running clap binary: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(currentPath)
	if err == nil {
		currentPath = resolved
	}

	stagingPath := filepath.Join(filepath.Dir(currentPath), ".clap-upgrade-tmp")
	if err := copyFile(newBin, stagingPath); err != nil {
		return fmt.Errorf("staging new binary next to %s: %w", currentPath, err)
	}
	if err := os.Chmod(stagingPath, 0o755); err != nil {
		_ = os.Remove(stagingPath)
		return err
	}
	if err := os.Rename(stagingPath, currentPath); err != nil {
		_ = os.Remove(stagingPath)
		return fmt.Errorf("replacing %s: %w (you may need to run this with elevated permissions)", currentPath, err)
	}
	return nil
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
