package aur

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func Upgrade() error {
	tracked := Tracked()
	if len(tracked) == 0 {
		fmt.Println("clap: no AUR packages tracked")
		return nil
	}

	names := make([]string, 0, len(tracked))
	for n := range tracked {
		names = append(names, n)
	}
	remote, err := Info(names)
	if err != nil {
		return fmt.Errorf("querying AUR for updates: %w", err)
	}
	byName := map[string]Package{}
	for _, p := range remote {
		byName[p.Name] = p
	}

	upgraded := 0
	for name, entry := range tracked {
		p, ok := byName[name]
		if !ok {
			fmt.Printf("clap: %s no longer found in the AUR, skipping\n", name)
			continue
		}
		cmp, err := vercmp(entry.Version, p.Version)
		if err != nil {
			fmt.Printf("clap: could not compare versions for %s (%v), skipping\n", name, err)
			continue
		}
		if cmp >= 0 {
			fmt.Printf("clap: %s up to date (%s)\n", name, entry.Version)
			continue
		}
		fmt.Printf("clap: upgrading %s: %s -> %s\n", name, entry.Version, p.Version)
		if err := Install(name, &InstallOptions{AsDep: entry.AsDep, NoConfirm: true}); err != nil {
			return fmt.Errorf("upgrading %s: %w", name, err)
		}
		upgraded++
	}

	fmt.Printf("clap: %d package(s) upgraded, %d already up to date\n", upgraded, len(tracked)-upgraded)
	return nil
}

func vercmp(a, b string) (int, error) {
	out, err := exec.Command("vercmp", a, b).Output()
	if err != nil {
		return 0, fmt.Errorf("vercmp not available or failed: %w", err)
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}
