package selfupgrade

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceRunningBinary(t *testing.T) {
	dir := t.TempDir()
	currentPath := filepath.Join(dir, "clap")
	if err := os.WriteFile(currentPath, []byte("old binary content"), 0o755); err != nil {
		t.Fatal(err)
	}

	newBin := filepath.Join(dir, "new-download")
	if err := os.WriteFile(newBin, []byte("new binary content"), 0o644); err != nil {
		t.Fatal(err)
	}

	origExecutable := executableFunc
	executableFunc = func() (string, error) { return currentPath, nil }
	defer func() { executableFunc = origExecutable }()

	if err := replaceRunningBinary(newBin); err != nil {
		t.Fatalf("replaceRunningBinary failed: %v", err)
	}

	data, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new binary content" {
		t.Fatalf("expected replaced content, got %q", data)
	}

	info, err := os.Stat(currentPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("expected replaced binary to be executable, got mode %v", info.Mode())
	}

	if _, err := os.Stat(filepath.Join(dir, ".clap-upgrade-tmp")); !os.IsNotExist(err) {
		t.Fatalf("staging file should have been renamed away, got err=%v", err)
	}
}

func TestReplaceRunningBinaryPreservesOldOnFailure(t *testing.T) {
	dir := t.TempDir()
	currentPath := filepath.Join(dir, "clap")
	if err := os.WriteFile(currentPath, []byte("old binary content"), 0o755); err != nil {
		t.Fatal(err)
	}

	origExecutable := executableFunc
	executableFunc = func() (string, error) { return currentPath, nil }
	defer func() { executableFunc = origExecutable }()

	err := replaceRunningBinary(filepath.Join(dir, "does-not-exist"))
	if err == nil {
		t.Fatal("expected an error for a missing source binary")
	}

	data, rerr := os.ReadFile(currentPath)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(data) != "old binary content" {
		t.Fatalf("old binary should be untouched on failure, got %q", data)
	}
}
