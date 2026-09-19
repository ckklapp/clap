package builder

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rokiri/clap/internal/manifest"
	"github.com/rokiri/clap/internal/pkgfile"
	"github.com/rokiri/clap/internal/store"
)

func ParseGitHubURL(u string) (owner, repo string, err error) {
	u = strings.TrimSpace(u)
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "github.com/")
	u = strings.TrimSuffix(u, ".git")
	u = strings.Trim(u, "/")
	parts := strings.Split(u, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("could not parse owner/repo from %q", u)
	}
	return parts[0], parts[1], nil
}

func runCmd(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if name == "go" {
		cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	}
	return cmd.Run()
}

func gitDescribe(dir string) string {
	out, err := exec.Command("git", "-C", dir, "describe", "--tags", "--always").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func clone(owner, repo, dest, tag string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return errors.New("git is required but was not found in PATH")
	}
	_ = os.RemoveAll(dest)
	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	args := []string{"clone", "--depth", "1"}
	if tag != "" {
		args = append(args, "--branch", tag)
	}
	args = append(args, repoURL, dest)
	fmt.Printf("clap: git clone %s\n", repoURL)
	return runCmd("", "git", args...)
}

func findMainDir(src, hint string) (string, error) {
	if hasMainPackage(src) {
		return src, nil
	}
	var candidates []string
	_ = filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == "vendor" || base == "testdata" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".go") && looksLikeMain(p) {
			candidates = append(candidates, filepath.Dir(p))
		}
		return nil
	})
	if len(candidates) == 0 {
		return "", fmt.Errorf("no \"package main\" found anywhere in the repo")
	}
	h := strings.ToLower(hint)
	for _, c := range candidates {
		if strings.ToLower(filepath.Base(c)) == h {
			return c, nil
		}
	}
	for _, c := range candidates {
		if strings.Contains(strings.ToLower(c), string(os.PathSeparator)+"cmd"+string(os.PathSeparator)) {
			return c, nil
		}
	}
	return candidates[0], nil
}

func hasMainPackage(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		if looksLikeMain(filepath.Join(dir, e.Name())) {
			return true
		}
	}
	return false
}

func looksLikeMain(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	s := string(data)
	return strings.Contains(s, "package main") && strings.Contains(s, "func main(")
}

func FromLocalSource(path, name string) error {
	if _, err := exec.LookPath("go"); err != nil {
		return errors.New("go toolchain is required to build from source but was not found in PATH")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil {
		if _, ferr := findMainDir(abs, name); ferr != nil {
			return fmt.Errorf("no go.mod found under %s and no package main detected either", abs)
		}
	}
	buildDir, err := findMainDir(abs, name)
	if err != nil {
		return err
	}
	binName := name
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	outBin := filepath.Join(store.TmpDir(), binName)
	_ = os.Remove(outBin)
	fmt.Printf("clap: go build (in %s)\n", buildDir)
	if err := runCmd(abs, "go", "build", "-o", outBin, "./"+relOrDot(abs, buildDir)); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}
	_ = os.Chmod(outBin, 0o755)
	tag := gitDescribe(abs)
	m := manifest.Manifest{
		Name: name, Repo: "local:" + abs, Tag: tag,
		Entry: name, OS: runtime.GOOS, Arch: runtime.GOARCH,
		Source: "local-build", Kind: "app", ClapVer: manifest.Version,
	}
	dest := filepath.Join(store.AppsDir(), name+".clap")
	if err := pkgfile.Package(dest, outBin, name, m); err != nil {
		return err
	}
	fmt.Printf("clap: built and installed %q -> %s\n", name, dest)
	return nil
}

func IsLocalPath(url string) (string, bool) {
	if strings.HasPrefix(url, "file://") {
		return strings.TrimPrefix(url, "file://"), true
	}
	if strings.HasPrefix(url, "/") || strings.HasPrefix(url, "./") || strings.HasPrefix(url, "../") || url == "." {
		if info, err := os.Stat(url); err == nil && info.IsDir() {
			return url, true
		}
	}
	return "", false
}

func FromSource(owner, repo, name, tag string) error {
	if _, err := exec.LookPath("go"); err != nil {
		return errors.New("go toolchain is required to build from source but was not found in PATH")
	}
	src := filepath.Join(store.TmpDir(), "src-"+name)
	if err := clone(owner, repo, src, tag); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}
	if _, err := os.Stat(filepath.Join(src, "go.mod")); err != nil {
		return fmt.Errorf("repo does not look like a Go module (no go.mod found), cannot build automatically")
	}
	buildDir, err := findMainDir(src, name)
	if err != nil {
		return err
	}
	binName := name
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	outBin := filepath.Join(store.TmpDir(), binName)
	_ = os.Remove(outBin)
	fmt.Printf("clap: go build (in %s)\n", strings.TrimPrefix(buildDir, src))
	if err := runCmd(src, "go", "build", "-o", outBin, "./"+relOrDot(src, buildDir)); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}
	_ = os.Chmod(outBin, 0o755)
	resolvedTag := tag
	if resolvedTag == "" {
		resolvedTag = gitDescribe(src)
	}
	m := manifest.Manifest{
		Name: name, Repo: owner + "/" + repo, Tag: resolvedTag,
		Entry: name, OS: runtime.GOOS, Arch: runtime.GOARCH,
		Source: "build", Kind: "app", ClapVer: manifest.Version,
	}
	dest := filepath.Join(store.AppsDir(), name+".clap")
	if err := pkgfile.Package(dest, outBin, name, m); err != nil {
		return err
	}
	fmt.Printf("clap: built and installed %q -> %s\n", name, dest)
	return nil
}

func relOrDot(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == "." {
		return "."
	}
	return filepath.ToSlash(rel)
}

func VendorLibrary(owner, repo, name, tag string) error {
	src := filepath.Join(store.TmpDir(), "src-"+name)
	if err := clone(owner, repo, src, tag); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}
	resolvedTag := tag
	if resolvedTag == "" {
		resolvedTag = gitDescribe(src)
	}
	m := manifest.Manifest{
		Name: name, Repo: owner + "/" + repo, Tag: resolvedTag,
		Entry: "", OS: "any", Arch: "any",
		Source: "source", Kind: "library", ClapVer: manifest.Version,
	}
	dest := filepath.Join(store.AppsDir(), name+".clapl")
	if err := pkgfile.PackageDir(dest, src, m); err != nil {
		return err
	}
	fmt.Printf("clap: vendored and installed library %q -> %s\n", name, dest)
	return nil
}
