package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rokiri/clap/internal/aur"
	"github.com/rokiri/clap/internal/builder"
	"github.com/rokiri/clap/internal/config"
	"github.com/rokiri/clap/internal/gitinstall"
	"github.com/rokiri/clap/internal/installer"
	"github.com/rokiri/clap/internal/manifest"
	"github.com/rokiri/clap/internal/pkgfile"
	"github.com/rokiri/clap/internal/project"
	"github.com/rokiri/clap/internal/provider"
	"github.com/rokiri/clap/internal/registry"
	"github.com/rokiri/clap/internal/selfupgrade"
	"github.com/rokiri/clap/internal/snap"
	"github.com/rokiri/clap/internal/store"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	if args[0] == "-c" {
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "clap: error: usage: clap -c <self-command>  (supported: upgrade)")
			os.Exit(1)
		}
		if err := dispatchSelf(args[1], args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "clap: error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if args[0] == "--app" {
		args = append([]string{"app"}, args[1:]...)
	}

	cmd := strings.TrimLeft(args[0], "-")
	rest := args[1:]

	var err error
	switch cmd {
	case "install", "i":
		err = cmdInstall(rest)
	case "search", "s":
		err = cmdSearch(rest)
	case "upgrade", "u", "-Syu":
		err = cmdUpgrade(rest)
	case "providers":
		fmt.Print(provider.List())
	case "build":
		err = cmdBuild(rest)
	case "app", "run", "open":
		err = cmdApp(rest)
	case "get", "use":
		err = cmdGet(rest)
	case "list", "ls":
		err = cmdList(rest)
	case "remove", "rm", "uninstall":
		err = cmdRemove(rest)
	case "info", "show":
		err = cmdInfo(rest)
	case "init":
		err = project.Init(".")
	case "sync":
		err = project.Sync(".")
	case "help", "h":
		printUsage()
	case "version", "v":
		fmt.Println("clap", manifest.Version)
	default:
		fmt.Fprintf(os.Stderr, "clap: unknown command %q\n\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "clap: error: %v\n", err)
		os.Exit(1)
	}
}

func dispatchSelf(cmd string, rest []string) error {
	switch strings.TrimLeft(cmd, "-") {
	case "upgrade", "u":
		return selfupgrade.Upgrade()
	case "version", "v":
		fmt.Println("clap", manifest.Version)
		return nil
	case "provider":
		flags, _ := parseFlags(rest)
		raw := flags["provider"]
		if raw == "" {
			return errors.New("invalid: --provider is required, e.g. `clap -c provider --provider=github`")
		}
		p, err := provider.Parse(raw)
		if err != nil {
			return err
		}
		if err := config.SetDefaultProvider(string(p)); err != nil {
			return err
		}
		fmt.Printf("clap: default provider set to %s\n", p)
		return nil
	default:
		return fmt.Errorf("unknown self-command %q, supported: upgrade, version, provider", cmd)
	}
}

func printUsage() {
	fmt.Print(`clap - a tiny Go-based package manager for GitHub apps

Usage:
  clap -c upgrade                                    upgrade clap itself (from ckklapp/clappm)
  clap -c version                                     print clap's own version
  clap -c provider --provider=<name>                 set the default provider (required)
  clap install <package>                             install from the default provider (aur, unless changed)
  clap install --provider=<name> <package>            install from a specific provider (aur, snap)
  clap install --url=<repo-url> [flags]              install directly from any git repo (any host)
  clap providers                                      list supported --provider values
  clap search <query>                                 search the AUR
  clap upgrade                                        upgrade all AUR/git/snap-tracked packages
  clap build --url=<github-repo-url> [--name=x] [--tag=x] [--lib=true]
  clap app <name> [-- args...]
  clap get <name> --to=<dir>
  clap list
  clap info <name>
  clap remove <name>
  clap init
  clap sync

Install flags:
  --name=<app>
  --tag=<tag>
  --asset=<glob>
  --os=<goos>
  --arch=<goarch>
  --no-verify=true
  --lib=true
`)
}

func parseFlags(args []string) (flags map[string]string, positional []string) {
	flags = map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			a = a[2:]
			if eq := strings.IndexByte(a, '='); eq >= 0 {
				flags[a[:eq]] = a[eq+1:]
				continue
			}
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				flags[a] = args[i+1]
				i++
			} else {
				flags[a] = "true"
			}
			continue
		}
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		positional = append(positional, a)
	}
	return
}

func cmdInstall(args []string) error {
	flags, pos := parseFlags(args)
	url := flags["url"]
	if url == "" {
		url = flags["repo"]
	}

	if url != "" {
		if flags["provider"] != "" {
			return fmt.Errorf("--url and --provider can't be combined — --url clones a repo directly regardless of provider, %q is not needed here", flags["provider"])
		}
		if flags["lib"] == "true" {
			_, err := installer.Install(installer.Options{
				URL: url, Name: flags["name"], Tag: flags["tag"], AssetGlob: flags["asset"],
				OS: flags["os"], Arch: flags["arch"], SkipVerify: flags["no-verify"] == "true", Lib: true,
			})
			return err
		}
		return gitinstall.Install(url, flags["name"], flags["tag"])
	}

	var prov provider.Provider
	if raw := flags["provider"]; raw != "" {
		p, err := provider.Parse(raw)
		if err != nil {
			return err
		}
		prov = p
	} else if def, ok := config.DefaultProvider(); ok {
		p, err := provider.Parse(def)
		if err != nil {
			return fmt.Errorf("saved default provider %q is invalid (%w) — reset it with `clap -c provider --provider=<name>`", def, err)
		}
		prov = p
	} else {
		prov = provider.Default
	}

	if len(pos) == 0 {
		return fmt.Errorf("usage: clap install <package>  (resolved against provider %q — pass --url=<repo-url> instead for any other git host)", prov)
	}

	switch prov {
	case provider.AUR:
		pkgName := aur.ParsePackageName(pos[0])
		return aur.Install(pkgName, &aur.InstallOptions{NoConfirm: flags["noconfirm"] == "true"})

	case provider.Snap:
		if !snap.Available() {
			return errors.New("the `snap` command was not found on PATH — snapd does not appear to be installed")
		}
		pkgName := snap.ParsePackageName(pos[0])
		return snap.Install(pkgName, flags["classic"] == "true")

	default:
		return fmt.Errorf("provider %q is not wired up in cmdInstall (this is a bug)", prov)
	}
}

func cmdSearch(args []string) error {
	_, pos := parseFlags(args)
	if len(pos) == 0 {
		return errors.New("usage: clap search <query>")
	}
	pkgs, err := aur.Search(pos[0], "")
	if err != nil {
		return err
	}
	if len(pkgs) == 0 {
		fmt.Println("no results")
		return nil
	}
	for _, p := range pkgs {
		fmt.Printf("%s %s\n    %s\n", p.Name, p.Version, p.Description)
	}
	return nil
}

func cmdUpgrade(_ []string) error {
	if err := aur.Upgrade(); err != nil {
		return err
	}
	if err := gitinstall.Upgrade(); err != nil {
		return err
	}
	if snap.Available() {
		return snap.RefreshAll()
	}
	return nil
}

func cmdBuild(args []string) error {
	flags, _ := parseFlags(args)
	url := flags["url"]
	if url == "" {
		url = flags["repo"]
	}
	if url == "" {
		return errors.New("missing --url=<github-repo-url>")
	}
	owner, repo, err := builder.ParseGitHubURL(url)
	if err != nil {
		return err
	}
	name := flags["name"]
	if name == "" {
		name = repo
	}
	tag := flags["tag"]

	if err := os.MkdirAll(store.AppsDir(), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(store.TmpDir(), 0o755); err != nil {
		return err
	}

	if flags["lib"] == "true" {
		return builder.VendorLibrary(owner, repo, name, tag)
	}
	return builder.FromSource(owner, repo, name, tag)
}

func cmdApp(args []string) error {
	_, pos := parseFlags(args)
	if len(pos) == 0 {
		return errors.New("usage: clap app <name> [-- args...]")
	}
	name := pos[0]
	passArgs := pos[1:]

	clapPath, ext := store.Resolve(name)
	if clapPath == "" {
		return fmt.Errorf("app %q is not installed", name)
	}

	extractDir := filepath.Join(store.RunDir(), name)
	if err := pkgfile.Extract(clapPath, extractDir); err != nil {
		return fmt.Errorf("extracting %s: %w", clapPath, err)
	}

	m, err := readManifestFromDir(extractDir)
	if err != nil {
		return err
	}

	if ext == ".clapl" || m.Kind == "library" {
		return fmt.Errorf("%q is a library (.clapl), not a runnable app — use `clap get %s --to=<dir>` instead", name, name)
	}

	entry := filepath.Join(extractDir, m.Entry)
	if _, err := os.Stat(entry); err != nil {
		return fmt.Errorf("entry binary %q not found in package", m.Entry)
	}
	_ = os.Chmod(entry, 0o755)

	cmd := exec.Command(entry, passArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func readManifestFromDir(dir string) (manifest.Manifest, error) {
	var m manifest.Manifest
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, fmt.Errorf("reading manifest: %w", err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parsing manifest: %w", err)
	}
	return m, nil
}

func cmdGet(args []string) error {
	flags, pos := parseFlags(args)
	if len(pos) == 0 {
		return errors.New("usage: clap get <name> --to=<dir>")
	}
	name := pos[0]
	to := flags["to"]
	if to == "" {
		to = "./" + name
	}
	clapPath, _ := store.Resolve(name)
	if clapPath == "" {
		return fmt.Errorf("%q is not installed", name)
	}
	tmpExtract := filepath.Join(store.TmpDir(), "get-"+name)
	_ = os.RemoveAll(tmpExtract)
	if err := pkgfile.Extract(clapPath, tmpExtract); err != nil {
		return fmt.Errorf("extracting %s: %w", clapPath, err)
	}
	m, err := readManifestFromDir(tmpExtract)
	if err != nil {
		return err
	}
	payload := tmpExtract
	if m.Source == "source" {
		payload = filepath.Join(tmpExtract, "src")
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	if err := pkgfile.CopyDir(payload, to); err != nil {
		return fmt.Errorf("copying to %s: %w", to, err)
	}
	fmt.Printf("clap: extracted %q (%s) -> %s\n", name, m.Kind, to)
	return nil
}

func cmdList(_ []string) error {
	entries, err := os.ReadDir(store.AppsDir())
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("(no packages installed)")
			return nil
		}
		return err
	}
	re := regexp.MustCompile(`\.(clap|clapl)$`)
	found := false
	for _, e := range entries {
		if e.IsDir() || !re.MatchString(e.Name()) {
			continue
		}
		found = true
		kind := "app"
		if strings.HasSuffix(e.Name(), ".clapl") {
			kind = "library"
		}
		name := strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".clapl"), ".clap")
		fmt.Printf("%s\t(%s)\n", name, kind)
	}
	if !found {
		fmt.Println("(no packages installed)")
	}
	return nil
}

func cmdInfo(args []string) error {
	_, pos := parseFlags(args)
	if len(pos) == 0 {
		return errors.New("usage: clap info <name>")
	}
	name := pos[0]
	clapPath, _ := store.Resolve(name)
	if clapPath == "" {
		return fmt.Errorf("%q is not installed", name)
	}
	m, err := pkgfile.ReadManifest(clapPath)
	if err != nil {
		return err
	}
	fmt.Printf("name:    %s\n", m.Name)
	fmt.Printf("kind:    %s\n", m.Kind)
	fmt.Printf("repo:    %s\n", m.Repo)
	fmt.Printf("tag:     %s\n", m.Tag)
	fmt.Printf("os/arch: %s/%s\n", m.OS, m.Arch)
	fmt.Printf("source:  %s\n", m.Source)
	if m.Entry != "" {
		fmt.Printf("entry:   %s\n", m.Entry)
	}
	return nil
}

func cmdRemove(args []string) error {
	_, pos := parseFlags(args)
	if len(pos) == 0 {
		return errors.New("usage: clap remove <name>")
	}
	name := pos[0]

	if clapPath, _ := store.Resolve(name); clapPath != "" {
		if err := os.Remove(clapPath); err != nil {
			return fmt.Errorf("could not remove %q: %w", name, err)
		}
		_ = os.RemoveAll(filepath.Join(store.RunDir(), name))
		fmt.Printf("clap: removed %q\n", name)
		return nil
	}

	if _, ok := aur.Tracked()[name]; ok {
		return aur.Remove(name)
	}

	if snap.Available() && snap.IsInstalled(name) {
		return snap.Remove(name)
	}

	if _, ok := registry.Get(name); ok {
		return gitinstall.Remove(name)
	}

	return fmt.Errorf("%q is not installed", name)
}
