package pacmanrepo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rokiri/clap/internal/ghrelease"
)

func TestDetectFullRepoWithFiles(t *testing.T) {
	rel := &ghrelease.Release{
		Assets: []ghrelease.Asset{
			{Name: "myrepo.db.tar.gz"},
			{Name: "myrepo.db"},
			{Name: "myrepo.files.tar.gz"},
			{Name: "myrepo.files"},
			{Name: "mypkg-1.0.0-1-x86_64.pkg.tar.zst"},
		},
	}
	name, hasFiles, ok := Detect(rel)
	if !ok {
		t.Fatal("expected detection to succeed")
	}
	if name != "myrepo" {
		t.Fatalf("expected repo name 'myrepo', got %q", name)
	}
	if !hasFiles {
		t.Fatal("expected hasFiles=true")
	}
}

func TestDetectDBOnlyNoFiles(t *testing.T) {
	rel := &ghrelease.Release{
		Assets: []ghrelease.Asset{
			{Name: "myrepo.db"},
			{Name: "mypkg-1.0.0-1-x86_64.pkg.tar.zst"},
		},
	}
	name, hasFiles, ok := Detect(rel)
	if !ok {
		t.Fatal("expected detection to succeed even without .files")
	}
	if name != "myrepo" {
		t.Fatalf("expected repo name 'myrepo', got %q", name)
	}
	if hasFiles {
		t.Fatal("expected hasFiles=false")
	}
}

func TestDetectNoMatch(t *testing.T) {
	rel := &ghrelease.Release{
		Assets: []ghrelease.Asset{
			{Name: "mypkg-1.0.0-1-x86_64.pkg.tar.zst"},
			{Name: "checksums.txt"},
		},
	}
	if _, _, ok := Detect(rel); ok {
		t.Fatal("expected no detection for a release with only a lone package asset")
	}
}

func TestDetectDoesNotConfuseAppImageOrArchive(t *testing.T) {
	rel := &ghrelease.Release{
		Assets: []ghrelease.Asset{
			{Name: "myapp.AppImage"},
			{Name: "myapp.tar.gz"},
		},
	}
	if _, _, ok := Detect(rel); ok {
		t.Fatal("expected no false-positive detection")
	}
}

func TestAddRepoIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "pacman.conf")
	if err := os.WriteFile(confPath, []byte("[options]\nArchitecture = auto\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAP_PACMAN_CONF", confPath)

	present, err := alreadyRegistered("myrepo")
	if err != nil {
		t.Fatal(err)
	}
	if present {
		t.Fatal("expected myrepo not to be registered yet")
	}

	appendManually(t, confPath, "\n[myrepo]\nSigLevel = Optional TrustAll\nServer = https://example.com/\n")

	present, err = alreadyRegistered("myrepo")
	if err != nil {
		t.Fatal(err)
	}
	if !present {
		t.Fatal("expected myrepo to be detected as already registered")
	}
}

func appendManually(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

func TestAddRepoAppendsCorrectBlock(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "pacman.conf")
	if err := os.WriteFile(confPath, []byte("[options]\nArchitecture = auto\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAP_PACMAN_CONF", confPath)

	origAppend := appendToConf
	appendToConf = func(path, content string) error {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.WriteString(content)
		return err
	}
	defer func() { appendToConf = origAppend }()

	if err := AddRepo("customrepo", "https://github.com/owner/repo/releases/download/v1.0/"); err != nil {
		t.Fatalf("AddRepo failed: %v", err)
	}

	data, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "[customrepo]") {
		t.Errorf("expected [customrepo] section, got:\n%s", got)
	}
	if !strings.Contains(got, "Server = https://github.com/owner/repo/releases/download/v1.0/") {
		t.Errorf("expected Server line, got:\n%s", got)
	}
	if !strings.Contains(got, "SigLevel = Optional TrustAll") {
		t.Errorf("expected SigLevel line, got:\n%s", got)
	}

	callCount := 0
	appendToConf = func(path, content string) error {
		callCount++
		return nil
	}
	if err := AddRepo("customrepo", "https://example.com/"); err != nil {
		t.Fatalf("second AddRepo call failed: %v", err)
	}
	if callCount != 0 {
		t.Fatalf("expected AddRepo to skip appending for an already-registered repo, but appendToConf was called %d time(s)", callCount)
	}
}

func TestServerURL(t *testing.T) {
	got := ServerURL("owner", "repo", "v1.2.0")
	want := "https://github.com/owner/repo/releases/download/v1.2.0/"
	if got != want {
		t.Fatalf("ServerURL() = %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, "/") {
		t.Fatal("ServerURL must end with a trailing slash for pacman's Server= to append filenames correctly")
	}
}
