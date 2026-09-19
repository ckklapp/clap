package snap

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func ParsePackageName(input string) string {
	s := strings.TrimSpace(input)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimSuffix(s, "/")

	if i := strings.Index(s, "snapcraft.io/"); i >= 0 {
		s = s[i+len("snapcraft.io/"):]
	}

	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}

	return s
}

func run(args ...string) error {
	cmd := exec.Command("sudo", append([]string{"snap"}, args...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func IsInstalled(name string) bool {
	return exec.Command("snap", "list", name).Run() == nil
}

func Install(name string, classic bool) error {
	if IsInstalled(name) {
		fmt.Printf("clap: %s is already installed as a snap (use `clap upgrade` to update)\n", name)
		return nil
	}
	args := []string{"install", name}
	if classic {
		args = append(args, "--classic")
	}
	fmt.Printf("clap: sudo snap %s\n", strings.Join(args, " "))
	if err := run(args...); err != nil {
		return fmt.Errorf("snap install failed: %w", err)
	}
	return nil
}

func Remove(name string) error {
	fmt.Printf("clap: sudo snap remove %s\n", name)
	if err := run("remove", name); err != nil {
		return fmt.Errorf("snap remove failed: %w", err)
	}
	return nil
}

func RefreshAll() error {
	fmt.Println("clap: sudo snap refresh")
	if err := run("refresh"); err != nil {
		return fmt.Errorf("snap refresh failed: %w", err)
	}
	return nil
}

func Available() bool {
	_, err := exec.LookPath("snap")
	return err == nil
}
