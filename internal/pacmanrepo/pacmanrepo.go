package pacmanrepo

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/rokiri/clap/internal/ghrelease"
)

var dbSuffixes = []string{".db.tar.zst", ".db.tar.xz", ".db.tar.gz", ".db"}
var filesSuffixes = []string{".files.tar.zst", ".files.tar.xz", ".files.tar.gz", ".files"}

func stripSuffix(name string, suffixes []string) (base string, matched bool) {
	for _, suf := range suffixes {
		if strings.HasSuffix(name, suf) {
			return strings.TrimSuffix(name, suf), true
		}
	}
	return "", false
}

func Detect(rel *ghrelease.Release) (repoName string, hasFiles bool, ok bool) {
	dbBases := map[string]bool{}
	filesBases := map[string]bool{}
	for _, a := range rel.Assets {
		if base, matched := stripSuffix(a.Name, dbSuffixes); matched {
			dbBases[base] = true
			continue
		}
		if base, matched := stripSuffix(a.Name, filesSuffixes); matched {
			filesBases[base] = true
		}
	}
	if len(dbBases) == 0 {
		return "", false, false
	}
	for base := range dbBases {
		return base, filesBases[base], true
	}
	return "", false, false
}

func ServerURL(owner, repo, tag string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/", owner, repo, tag)
}

func conf() string {
	if c := os.Getenv("CLAP_PACMAN_CONF"); c != "" {
		return c
	}
	return "/etc/pacman.conf"
}

func alreadyRegistered(repoName string) (bool, error) {
	f, err := os.Open(conf())
	if err != nil {
		return false, err
	}
	defer f.Close()
	header := "[" + repoName + "]"
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == header {
			return true, nil
		}
	}
	return false, sc.Err()
}

var appendToConf = func(path, content string) error {
	cmd := exec.Command("sudo", "tee", "-a", path)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func AddRepo(repoName, serverURL string) error {
	present, err := alreadyRegistered(repoName)
	if err != nil {
		return fmt.Errorf("reading %s: %w", conf(), err)
	}
	if present {
		fmt.Printf("clap: repo [%s] is already in %s, not adding again\n", repoName, conf())
		return nil
	}

	block := fmt.Sprintf("\n[%s]\nSigLevel = Optional TrustAll\nServer = %s\n", repoName, serverURL)
	fmt.Printf("clap: adding [%s] to %s (Server = %s)\n", repoName, conf(), serverURL)
	fmt.Println("clap: note — SigLevel is set to \"Optional TrustAll\" since GitHub release assets aren't pacman-signed; only add repos you trust")

	if err := appendToConf(conf(), block); err != nil {
		return fmt.Errorf("appending to %s: %w", conf(), err)
	}
	return nil
}

func SyncAndInstall(pkgName string, asDep, noConfirm bool) error {
	if err := run("sudo", "pacman", "-Sy"); err != nil {
		return fmt.Errorf("pacman -Sy failed: %w", err)
	}
	args := []string{"pacman", "-S", "--needed"}
	if asDep {
		args = append(args, "--asdeps")
	}
	if noConfirm {
		args = append(args, "--noconfirm")
	}
	args = append(args, pkgName)
	if err := run("sudo", args...); err != nil {
		return fmt.Errorf("pacman -S %s failed: %w", pkgName, err)
	}
	return nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
