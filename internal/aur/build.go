package aur

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rokiri/clap/internal/store"
	"github.com/rokiri/clap/internal/sudoauth"
)

type InstallOptions struct {
	AsDep     bool
	NoConfirm bool

	KeepTmp bool

	CleanBuildDeps string

	seen    map[string]bool
	tracker *depTracker
}

type depTracker struct {
	infoCache      map[string]*Package
	installedAsDep map[string]bool
	viaMakeOnly    map[string]bool
	viaDepend      map[string]bool
	order          []string
	prereqsChecked bool
}

func newDepTracker() *depTracker {
	return &depTracker{
		infoCache:      map[string]*Package{},
		installedAsDep: map[string]bool{},
		viaMakeOnly:    map[string]bool{},
		viaDepend:      map[string]bool{},
	}
}

func Install(name string, opts *InstallOptions) error {
	if opts == nil {
		opts = &InstallOptions{}
	}
	isTopLevel := opts.seen == nil
	if opts.seen == nil {
		opts.seen = map[string]bool{}
	}
	if opts.tracker == nil {
		opts.tracker = newDepTracker()
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

	if isTopLevel {
		if err := sudoauth.Ensure(); err != nil {
			return err
		}
	}

	if inOfficialRepo(name) {
		fmt.Printf("clap: %s is in the official repos, installing via pacman\n", name)
		return pacmanSyncInstall(name, opts.AsDep)
	}

	if err := ensureBuildPrereqs(opts); err != nil {
		return fmt.Errorf("installing build prerequisites: %w", err)
	}

	pkg, err := infoCached(name, opts)
	if err != nil {
		if syncDBsLookEmpty() {
			return fmt.Errorf("%w (note: pacman's sync databases look empty/unsynced — if you expected this to be an official-repo package, run `sudo pacman -Sy` first and retry)", err)
		}
		return err
	}

	if err := installDeps(name, pkg, opts); err != nil {
		return err
	}

	buildDir := filepath.Join(store.TmpDir(), "aur-"+pkg.PackageBase)
	_ = os.RemoveAll(buildDir)
	if !opts.KeepTmp {

		defer func() { _ = os.RemoveAll(buildDir) }()
	}

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

	if opts.AsDep {
		opts.tracker.installedAsDep[pkg.Name] = true
		opts.tracker.order = append(opts.tracker.order, pkg.Name)
	}

	fmt.Printf("clap: installed %s %s\n", pkg.Name, pkg.Version)

	if isTopLevel {
		offerBuildDepCleanup(opts)
	}

	return nil
}

func installDeps(parent string, pkg *Package, opts *InstallOptions) error {
	type edge struct {
		name   string
		isMake bool
	}
	var edges []edge
	for _, raw := range pkg.Depends {
		if dep := StripVersionConstraint(raw); dep != "" {
			edges = append(edges, edge{dep, false})
		}
	}
	for _, raw := range pkg.MakeDepends {
		if dep := StripVersionConstraint(raw); dep != "" {
			edges = append(edges, edge{dep, true})
		}
	}

	var candidates []string
	seenEdge := map[string]bool{}
	for _, e := range edges {
		if e.isMake {
			if !opts.tracker.viaDepend[e.name] {
				opts.tracker.viaMakeOnly[e.name] = true
			}
		} else {
			opts.tracker.viaDepend[e.name] = true
			delete(opts.tracker.viaMakeOnly, e.name)
		}

		if seenEdge[e.name] || e.name == "" || isInstalled(e.name) || inOfficialRepo(e.name) {
			continue
		}
		seenEdge[e.name] = true
		candidates = append(candidates, e.name)
	}

	warmInfoCache(candidates, opts)

	for _, dep := range candidates {
		fmt.Printf("clap: resolving AUR dependency %s -> %s\n", parent, dep)
		if err := Install(dep, &InstallOptions{
			AsDep:     true,
			NoConfirm: opts.NoConfirm,
			KeepTmp:   opts.KeepTmp,
			seen:      opts.seen,
			tracker:   opts.tracker,
		}); err != nil {
			return fmt.Errorf("installing dependency %q of %q: %w", dep, parent, err)
		}
	}
	return nil
}

func warmInfoCache(names []string, opts *InstallOptions) {
	var need []string
	for _, n := range names {
		if _, ok := opts.tracker.infoCache[n]; !ok {
			need = append(need, n)
		}
	}
	if len(need) == 0 {
		return
	}
	pkgs, err := Info(need)
	if err != nil {

		return
	}
	for i := range pkgs {
		p := pkgs[i]
		opts.tracker.infoCache[p.Name] = &p
	}
}

func infoCached(name string, opts *InstallOptions) (*Package, error) {
	if p, ok := opts.tracker.infoCache[name]; ok {
		return p, nil
	}
	p, err := InfoOne(name)
	if err != nil {
		return nil, err
	}
	opts.tracker.infoCache[name] = p
	return p, nil
}

func ensureBuildPrereqs(opts *InstallOptions) error {
	if opts.tracker.prereqsChecked {
		return nil
	}
	opts.tracker.prereqsChecked = true

	var missing []string
	if !pacmanGroupInstalled("base-devel") {
		missing = append(missing, "base-devel")
	}
	if _, err := exec.LookPath("git"); err != nil {
		missing = append(missing, "git")
	}
	if len(missing) == 0 {
		return nil
	}

	fmt.Printf("clap: installing build prerequisites needed for AUR packages: %s\n", strings.Join(missing, ", "))
	args := []string{"pacman", "-S", "--needed"}
	if opts.NoConfirm {
		args = append(args, "--noconfirm")
	}
	args = append(args, missing...)
	return run("", "sudo", append([]string{}, args...)...)
}

func pacmanGroupInstalled(group string) bool {
	out, err := exec.Command("pacman", "-Qg", group).Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func offerBuildDepCleanup(opts *InstallOptions) {
	t := opts.tracker

	var candidates []string
	for _, n := range t.order {
		if t.installedAsDep[n] && t.viaMakeOnly[n] && !t.viaDepend[n] {
			candidates = append(candidates, n)
		}
	}
	if len(candidates) == 0 {
		return
	}

	orphans := orphanedPackages()
	var toOffer []string
	for _, n := range candidates {
		if orphans[n] {
			toOffer = append(toOffer, n)
		}
	}
	if len(toOffer) == 0 {
		return
	}

	fmt.Println("clap: these were only needed to build packages you just installed:")
	for i, n := range toOffer {
		fmt.Printf("  %d) %s\n", i+1, n)
	}

	answer := strings.ToLower(strings.TrimSpace(opts.CleanBuildDeps))
	if answer == "" {
		if opts.NoConfirm {
			answer = "n"
		} else {
			fmt.Print("Remove them now? [N]one / [A]ll / numbers e.g. 1,3 (default N): ")
			line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			answer = strings.ToLower(strings.TrimSpace(line))
		}
	}

	var toRemove []string
	switch answer {
	case "", "n", "none":
		return
	case "a", "all":
		toRemove = toOffer
	default:
		for _, tok := range strings.Split(answer, ",") {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}
			idx, err := strconv.Atoi(tok)
			if err != nil || idx < 1 || idx > len(toOffer) {
				fmt.Printf("clap: ignoring invalid selection %q\n", tok)
				continue
			}
			toRemove = append(toRemove, toOffer[idx-1])
		}
	}
	if len(toRemove) == 0 {
		return
	}

	args := []string{"pacman", "-Rns"}
	if opts.NoConfirm {
		args = append(args, "--noconfirm")
	}
	args = append(args, toRemove...)
	fmt.Printf("clap: sudo %s\n", strings.Join(args, " "))
	if err := run("", "sudo", append([]string{}, args...)...); err != nil {
		fmt.Printf("clap: warning — failed to remove build-only dependencies: %v\n", err)
		return
	}
	for _, n := range toRemove {
		_ = forgetInstall(n)
	}
}

func orphanedPackages() map[string]bool {
	set := map[string]bool{}
	out, err := exec.Command("pacman", "-Qtdq").Output()
	if err != nil {

		return set
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			set[line] = true
		}
	}
	return set
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
