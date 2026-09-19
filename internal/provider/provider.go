package provider

import (
	"fmt"
	"sort"
	"strings"
)

type Provider string

const (
	AUR  Provider = "aur"
	Snap Provider = "snap"
)

const Default = AUR

var implemented = map[Provider]bool{
	AUR:  true,
	Snap: true,
}

var descriptions = map[Provider]string{
	AUR:  "Arch User Repository — bare package name or https://aur.archlinux.org/packages/<pkg>",
	Snap: "Snap Store — bare package name or https://snapcraft.io/<pkg>",
}

func Parse(s string) (Provider, error) {
	p := Provider(strings.ToLower(strings.TrimSpace(s)))
	if _, known := descriptions[p]; !known {
		if p == "github" || p == "gitlab" || p == "git" {
			return "", fmt.Errorf("%q is not a provider — use --url=<repo-url> directly, it works for any git host", s)
		}
		return "", fmt.Errorf("unknown provider %q, supported: %s", s, strings.Join(Names(), ", "))
	}
	if !implemented[p] {
		return "", fmt.Errorf("provider %q is recognized but not yet implemented", p)
	}
	return p, nil
}

func Names() []string {
	names := make([]string, 0, len(descriptions))
	for p := range descriptions {
		names = append(names, string(p))
	}
	sort.Strings(names)
	return names
}

func List() string {
	var b strings.Builder
	names := Names()
	for _, n := range names {
		p := Provider(n)
		status := "implemented"
		if !implemented[p] {
			status = "planned, not yet implemented"
		}
		if p == Default {
			status += ", default"
		}
		fmt.Fprintf(&b, "  %-8s %s (%s)\n", n, descriptions[p], status)
	}
	b.WriteString("  (any other git host: use --url=<repo-url> directly, no --provider needed)\n")
	return b.String()
}
