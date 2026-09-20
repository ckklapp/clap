package sudoauth

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func Ensure() error {
	if _, err := exec.LookPath("sudo"); err != nil {
		return errors.New("sudo was not found on PATH — clap needs it to install packages")
	}
	cmd := exec.Command("sudo", "-v")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo authentication failed: %w", err)
	}
	return nil
}
