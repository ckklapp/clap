package gitinstall

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rokiri/clap/internal/appimage"
	"github.com/rokiri/clap/internal/archpkg"
	"github.com/rokiri/clap/internal/aur"
	"github.com/rokiri/clap/internal/ghrelease"
	"github.com/rokiri/clap/internal/pacmanrepo"
	"github.com/rokiri/clap/internal/registry"
	"github.com/rokiri/clap/internal/store"
	"github.com/rokiri/clap/internal/sudoauth"
)

func Normalize(input string) (cloneURL, host, name string, err error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", "", "", errors.New("empty --url")
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, perr := url.Parse(s)
	if perr != nil {
		return "", "", "", fmt.Errorf("invalid --url: %w", perr)
	}
	if u.Host == "" || !strings.Contains(u.Host, ".") {
		return "", "", "", fmt.Errorf("--url must point directly to a repo, e.g. https://github.com/owner/repo (got %q)", input)
	}
	path := strings.Trim(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	segments := strings.Split(path, "/")
	if len(segments) < 2 || segments[0] == "" || segments[1] == "" {
		return "", "", "", fmt.Errorf("--url must point directly to a repo (host/owner/repo), got %q", input)
	}
	name = segments[len(segments)-1]
	cloneURL = fmt.Sprintf("https://%s/%s.git", u.Host, path)
	return cloneURL, u.Host, name, nil
}

func githubOwnerRepo(cloneURL string) (string, string) {
	trimmed := strings.TrimPrefix(cloneURL, "https://github.com/")
	trimmed = strings.TrimSuffix(trimmed, ".git")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func Install(rawURL, name, tag string, addPacmanRepo bool) error {
	cloneURL, host, inferredName, err := Normalize(rawURL)
	if err != nil {
		return err
	}
	if name == "" {
		name = inferredName
	}

	if host == "github.com" {
		owner, repo := githubOwnerRepo(cloneURL)
		fmt.Printf("clap: resolving release for %s/%s...\n", owner, repo)
		rel, err := ghrelease.Fetch(owner, repo, tag)
		if err != nil {
			fmt.Printf("clap: could not fetch release info (%v), falling back to PKGBUILD-based build\n", err)
		} else {
			if repoName, hasFiles, ok := pacmanrepo.Detect(rel); ok {
				filesNote := ""
				if hasFiles {
					filesNote = " + .files"
				}
				if !addPacmanRepo {
					fmt.Printf("clap: release %s looks like a full pacman repo (found %s.db%s) — pass --add-pacman-repo to register it in pacman.conf instead of grabbing a single package; falling back to a one-off install for now\n", rel.TagName, repoName, filesNote)
				} else {
					fmt.Printf("clap: release %s looks like a full pacman repo (found %s.db%s) — registering it like a `pacman.conf` repo instead of grabbing a single package\n", rel.TagName, repoName, filesNote)
					if err := sudoauth.Ensure(); err != nil {
						return err
					}
					return installPacmanRepo(owner, repo, repoName, rel.TagName, rawURL, name)
				}
			}
			if a := archpkg.FindAsset(rel); a != nil {
				fmt.Printf("clap: found Arch package asset %q in release %s\n", a.Name, rel.TagName)
				if err := sudoauth.Ensure(); err != nil {
					return err
				}
				return installArchPkgAsset(a, rawURL, name, rel.TagName)
			}
			if a := appimage.FindAsset(rel); a != nil {
				fmt.Printf("clap: found AppImage asset %q in release %s\n", a.Name, rel.TagName)
				return installAppImageAsset(a, rawURL, name, rel.TagName)
			}
			fmt.Println("clap: no AppImage or Arch package asset in this release")
			if tag == "" {
				tag = rel.TagName
			}
		}
	} else {
		fmt.Printf("clap: %s is not github.com — skipping release-asset lookup (GitHub Releases only), going straight to PKGBUILD-based build\n", host)
	}

	return pkgbuildFallback(cloneURL, name, tag)
}

func installPacmanRepo(owner, repo, repoName, tag, rawURL, pkgName string) error {
	serverURL := pacmanrepo.ServerURL(owner, repo, tag)
	if err := pacmanrepo.AddRepo(repoName, serverURL); err != nil {
		return err
	}
	if err := pacmanrepo.SyncAndInstall(pkgName, false, true); err != nil {
		return err
	}
	return registry.Record(registry.Entry{Name: pkgName, URL: rawURL, Tag: tag, Kind: registry.KindPacmanRepo, Path: repoName})
}

func installArchPkgAsset(a *ghrelease.Asset, rawURL, name, tag string) error {
	dl := filepath.Join(store.BuildDir(name), a.Name)
	if err := os.MkdirAll(filepath.Dir(dl), 0o755); err != nil {
		return err
	}
	if err := ghrelease.Download(a.BrowserDownloadURL, dl); err != nil {
		return fmt.Errorf("downloading asset: %w", err)
	}
	if err := archpkg.Install(dl); err != nil {
		return err
	}
	return registry.Record(registry.Entry{Name: name, URL: rawURL, Tag: tag, Kind: registry.KindArchPkg, Path: dl})
}

func installAppImageAsset(a *ghrelease.Asset, rawURL, name, tag string) error {
	dl := filepath.Join(store.BuildDir(name), a.Name)
	if err := os.MkdirAll(filepath.Dir(dl), 0o755); err != nil {
		return err
	}
	if err := ghrelease.Download(a.BrowserDownloadURL, dl); err != nil {
		return fmt.Errorf("downloading asset: %w", err)
	}
	dest, err := appimage.Install(name, dl)
	if err != nil {
		return err
	}
	return registry.Record(registry.Entry{Name: name, URL: rawURL, Tag: tag, Kind: registry.KindAppImage, Path: dest})
}

func pkgbuildFallback(cloneURL, name, tag string) error {
	buildDir := store.BuildDir(name)
	_ = os.RemoveAll(buildDir)
	args := []string{"clone", "--depth", "1"}
	if tag != "" {
		args = append(args, "--branch", tag)
	}
	args = append(args, cloneURL, buildDir)
	fmt.Printf("clap: git clone %s\n", cloneURL)
	if err := runCmd("", "git", args...); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	if _, err := os.Stat(filepath.Join(buildDir, "PKGBUILD")); err == nil {
		fmt.Println("clap: PKGBUILD found in repo, building with makepkg")
		if err := sudoauth.Ensure(); err != nil {
			return err
		}
		return buildWithMakepkg(buildDir, name)
	}

	fmt.Printf("clap: no PKGBUILD in this repo, checking whether the AUR has a package named %q...\n", name)
	_ = os.RemoveAll(buildDir)
	if _, err := aur.InfoOne(name); err != nil {
		return fmt.Errorf("no AppImage/Arch package asset, no PKGBUILD in the repo, and no AUR package named %q either — clap only installs via a prebuilt asset or a PKGBUILD, it does not compile arbitrary source itself. Try `clap install %s` (AUR) if the package has a different name there", name, name)
	}
	fmt.Printf("clap: found AUR package %q, installing from there instead\n", name)
	return aur.Install(name, &aur.InstallOptions{NoConfirm: true})
}

func buildWithMakepkg(buildDir, name string) error {
	if err := runCmd(buildDir, "makepkg", "-s", "--needed", "--noconfirm"); err != nil {
		return fmt.Errorf("makepkg failed: %w", err)
	}
	matches, err := filepath.Glob(filepath.Join(buildDir, "*.pkg.tar.zst"))
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		matches, err = filepath.Glob(filepath.Join(buildDir, "*.pkg.tar.xz"))
		if err != nil {
			return err
		}
	}
	if len(matches) == 0 {
		return fmt.Errorf("makepkg reported success but no package file was found in %s", buildDir)
	}
	if err := installPkgFiles(matches); err != nil {
		return err
	}
	return registry.Record(registry.Entry{Name: name, Kind: registry.KindArchPkg, Path: matches[0]})
}

func installPkgFiles(files []string) error {
	args := append([]string{"pacman", "-U", "--noconfirm"}, files...)
	fmt.Printf("clap: sudo %s\n", strings.Join(args, " "))
	return runCmd("", "sudo", args...)
}

func runCmd(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func Upgrade() error {
	entries := registry.All()
	if len(entries) == 0 {
		fmt.Println("clap: no git-installed packages tracked")
		return nil
	}
	upgraded := 0
	for name, e := range entries {
		if e.URL == "" {
			continue
		}
		cloneURL, host, _, err := Normalize(e.URL)
		if err != nil || host != "github.com" {
			fmt.Printf("clap: %s is not a github.com package, skipping automatic upgrade check\n", name)
			continue
		}
		owner, repo := githubOwnerRepo(cloneURL)
		rel, err := ghrelease.Fetch(owner, repo, "")
		if err != nil {
			fmt.Printf("clap: could not check %s for updates (%v), skipping\n", name, err)
			continue
		}
		if rel.TagName == e.Tag {
			fmt.Printf("clap: %s up to date (%s)\n", name, e.Tag)
			continue
		}
		fmt.Printf("clap: upgrading %s: %s -> %s\n", name, e.Tag, rel.TagName)
		if err := Install(e.URL, name, "", e.Kind == registry.KindPacmanRepo); err != nil {
			return fmt.Errorf("upgrading %s: %w", name, err)
		}
		upgraded++
	}
	fmt.Printf("clap: %d git-tracked package(s) upgraded, %d already up to date\n", upgraded, len(entries)-upgraded)
	return nil
}

func Remove(name string) error {
	e, ok := registry.Get(name)
	if !ok {
		return fmt.Errorf("%q is not tracked by clap's git-install registry", name)
	}
	switch e.Kind {
	case registry.KindArchPkg, registry.KindPacmanRepo:
		if err := runCmd("", "sudo", "pacman", "-R", name); err != nil {
			return fmt.Errorf("pacman -R failed: %w", err)
		}
	case registry.KindAppImage:
		_ = os.Remove(filepath.Join(store.LocalBinDir(), name))
		_ = os.Remove(e.Path)
	case registry.KindBuild:
		_ = os.Remove(e.Path)
	}
	return registry.Forget(name)
}
