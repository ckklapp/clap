package ghrelease

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

func ghGet(url string, out interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "clap")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API %s returned %d: %s", url, resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func Fetch(owner, repo, tag string) (*Release, error) {
	var url string
	if tag == "" {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	} else {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", owner, repo, tag)
	}
	var rel Release
	if err := ghGet(url, &rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func FindPackageAsset(owner, repo, tag, assetGlob, ext string) (*Release, *Asset, error) {
	rel, err := Fetch(owner, repo, tag)
	if err != nil {
		return nil, nil, err
	}
	for i := range rel.Assets {
		a := &rel.Assets[i]
		if assetGlob != "" {
			if ok, _ := path.Match(assetGlob, a.Name); ok {
				return rel, a, nil
			}
			continue
		}
		if strings.HasSuffix(a.Name, ext) {
			return rel, a, nil
		}
	}
	return rel, nil, nil
}

var osAliases = map[string][]string{
	"linux":   {"linux"},
	"darwin":  {"darwin", "macos", "mac-os", "osx", "apple"},
	"windows": {"windows", "win64", "win32", ".exe"},
}

var archAliases = map[string][]string{
	"amd64": {"amd64", "x86_64", "x64"},
	"arm64": {"arm64", "aarch64"},
	"386":   {"386", "i386", "x86", "ia32"},
	"arm":   {"armv7", "armv6", "armhf", "arm-"},
}

var ignorePatterns = []string{
	".sha256", ".sha512", ".sig", ".asc", ".pem", ".sbom",
	"checksums", "checksum", "sha256sums", "sha512sums",
	".deb", ".rpm", ".apk", ".msi", ".dmg", ".pkg",
	"src.tar", "-source", "_source", "sources.",
}

func scoreAsset(name, goos, goarch string) int {
	lower := strings.ToLower(name)
	for _, pat := range ignorePatterns {
		if strings.Contains(lower, pat) {
			return -1
		}
	}
	score := 0
	osMatch := false
	for _, o := range osAliases[goos] {
		if strings.Contains(lower, o) {
			osMatch = true
			break
		}
	}
	if !osMatch {
		return -1
	}
	score += 20
	archMatch := false
	for _, ar := range archAliases[goarch] {
		if strings.Contains(lower, ar) {
			archMatch = true
			break
		}
	}
	if !archMatch {
		return -1
	}
	score += 20
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		score += 5
	case strings.HasSuffix(lower, ".zip"):
		score += 5
	case strings.HasSuffix(lower, ".exe"), !strings.Contains(path.Base(lower), "."):
		score += 6
	default:
		score += 1
	}
	return score
}

func FindBinaryAsset(rel *Release, goos, goarch string) *Asset {
	best := -1
	var bestAsset *Asset
	for i := range rel.Assets {
		a := &rel.Assets[i]
		s := scoreAsset(a.Name, goos, goarch)
		if s > best {
			best = s
			bestAsset = a
		}
	}
	if best < 0 {
		return nil
	}
	return bestAsset
}

func FindChecksumAsset(rel *Release) *Asset {
	candidates := []string{"checksums.txt", "sha256sums.txt", "sha256sums", "checksums"}
	for i := range rel.Assets {
		lower := strings.ToLower(rel.Assets[i].Name)
		for _, c := range candidates {
			if lower == c || strings.Contains(lower, c) {
				return &rel.Assets[i]
			}
		}
	}
	return nil
}

func VerifyChecksum(checksumURL, assetName, filePath string) (bool, error) {
	req, err := http.NewRequest("GET", checksumURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", "clap")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false, fmt.Errorf("HTTP %d fetching checksums", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	var want string
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		sum, fname := fields[0], strings.TrimPrefix(fields[1], "*")
		if fname == assetName || filepath.Base(fname) == assetName {
			want = strings.ToLower(sum)
			break
		}
	}
	if want == "" {
		return false, nil
	}
	f, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return false, fmt.Errorf("checksum mismatch: want %s, got %s", want, got)
	}
	return true, nil
}

func Download(url, dest string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "clap")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}
