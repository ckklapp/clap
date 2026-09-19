package aur

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rokiri/clap/internal/store"
)

type InstallOptions struct {
	AsDep     bool
	NoConfirm bool
	seen      map[string]bool
}

func Install(name string, opts *InstallOptions) error {
	if opts == nil {
		opts = &InstallOptions{}
	}
	isTopLevel := opts.seen == nil
	if opts.seen == nil {
		opts.seen = map[string]bool{}
	}
	if opts.seen[name] {
		return nil
	}
	opts.seen[name] = true

	if isTopLevel && syncDBsLookEmpty() {
		fmt.Println("clap: warning — pacman's sync databases look empty or missing. Packages that are actually in the official repos may wrongly appear unavailable. Run `sudo pacman -Sy` (or `-Syu`) first if installs fail unexpectedly.")
	}

	if isInstalled(name) && !opts.AsDep {
		fmt.Printf("clap: %s is already installed (use `clap upgrade` to update)\n", name)
		return nil
	}

	if inOfficialRepo(name) {
		fmt.Printf("clap: %s is in the official repos, installing via pacman\n", name)
		return pacmanSyncInstall(name, opts.AsDep)
	}

	pkg, err := InfoOne(name)
	if err != nil {
		if syncDBsLookEmpty() {
			return fmt.Errorf("%w (note: pacman's sync databases look empty/unsynced — if you expected this to be an official-repo package, run `sudo pacman -Sy` first and retry)", err)
		}
		return err
	}

	deps := append(append([]string{}, pkg.Depends...), pkg.MakeDepends...)
	for _, raw := range deps {
		dep := StripVersionConstraint(raw)
		if dep == "" || isInstalled(dep) || inOfficialRepo(dep) {
			continue
		}
		fmt.Printf("clap: resolving AUR dependency %s -> %s\n", name, dep)
		if err := Install(dep, &InstallOptions{AsDep: true, NoConfirm: opts.NoConfirm, seen: opts.seen}); err != nil {
			return fmt.Errorf("installing dependency %q of %q: %w", dep, name, err)
		}
	}

	buildDir := filepath.Join(store.TmpDir(), "aur-"+pkg.PackageBase)
	_ = os.RemoveAll(buildDir)

	repoURL := fmt.Sprintf("https://aur.archlinux.org/%s.git", pkg.PackageBase)
	fmt.Printf("clap: git clone %s\n", repoURL)
	if err := run("", "git", "clone", "--depth", "1", repoURL, buildDir); err != nil {
		return fmt.Errorf("cloning AUR repo: %w", err)
	}

	makepkgArgs := []string{"-s", "--needed"}
	if opts.NoConfirm {
		makepkgArgs = append(makepkgArgs, "--noconfirm")
	}
	fmt.Printf("clap: makepkg %s (in %s)\n", strings.Join(makepkgArgs, " "), buildDir)
	if err := run(buildDir, "makepkg", makepkgArgs...); err != nil {
		return fmt.Errorf("makepkg failed: %w", err)
	}

	pkgFiles, err := builtPackageFiles(buildDir)
	if err != nil {
		return err
	}
	if len(pkgFiles) == 0 {
		return fmt.Errorf("makepkg reported success but no package file was found in %s", buildDir)
	}

	pacmanArgs := []string{"-U"}
	if opts.AsDep {
		pacmanArgs = append(pacmanArgs, "--asdeps")
	}
	if opts.NoConfirm {
		pacmanArgs = append(pacmanArgs, "--noconfirm")
	}
	pacmanArgs = append(pacmanArgs, pkgFiles...)
	fmt.Printf("clap: sudo pacman %s\n", strings.Join(pacmanArgs, " "))
	if err := run("", "sudo", append([]string{"pacman"}, pacmanArgs...)...); err != nil {
		return fmt.Errorf("pacman -U failed: %w", err)
	}

	if err := recordInstall(DBEntry{
		Name: pkg.Name, PackageBase: pkg.PackageBase, Version: pkg.Version, AsDep: opts.AsDep,
	}); err != nil {
		return fmt.Errorf("recording install: %w", err)
	}

	fmt.Printf("clap: installed %s %s\n", pkg.Name, pkg.Version)
	return nil
}

func Remove(name string) error {
	if err := run("", "sudo", "pacman", "-R", name); err != nil {
		return fmt.Errorf("pacman -R failed: %w", err)
	}
	return forgetInstall(name)
}

func run(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func inOfficialRepo(name string) bool {
	return exec.Command("pacman", "-Si", name).Run() == nil
}

func isInstalled(name string) bool {
	return exec.Command("pacman", "-Qi", name).Run() == nil
}

func syncDBsLookEmpty() bool {
	matches, err := filepath.Glob("/var/lib/pacman/sync/*.db")
	if err != nil {
		return false
	}
	return len(matches) == 0
}

func pacmanSyncInstall(name string, asDep bool) error {
	args := []string{"-S", "--needed", "--noconfirm"}
	if asDep {
		args = append(args, "--asdeps")
	}
	args = append(args, name)
	return run("", "sudo", append([]string{"pacman"}, args...)...)
}

func builtPackageFiles(dir string) ([]string, error) {
	out, err := exec.Command("bash", "-c", "cd "+shellQuote(dir)+" && makepkg --packagelist").Output()
	if err != nil {
		return globPackages(dir)
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	if len(files) > 0 {
		return files, nil
	}
	return globPackages(dir)
}

func globPackages(dir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.pkg.tar.zst"))
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		matches, err = filepath.Glob(filepath.Join(dir, "*.pkg.tar.xz"))
		if err != nil {
			return nil, err
		}
	}
	return matches, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
