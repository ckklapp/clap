package archivex

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type candidate struct {
	name       string
	size       int64
	executable bool
}

func ExtractBinary(archivePath, destPath, hint string) error {
	lower := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return fromZip(archivePath, destPath, hint)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return fromTarGz(archivePath, destPath, hint)
	}
	return fmt.Errorf("unsupported archive format: %s", archivePath)
}

func pickBest(cands []candidate, hint string) string {
	skipExt := map[string]bool{
		".md": true, ".txt": true, ".1": true, ".man": true, ".desktop": true,
		".png": true, ".svg": true, ".yaml": true, ".yml": true, ".json": true,
		".toml": true, ".sh": true, ".bash": true, ".zsh": true, ".fish": true,
	}
	skipBase := map[string]bool{
		"license": true, "license.txt": true, "readme": true, "readme.md": true,
		"changelog": true, "changelog.md": true, "notice": true, "copying": true,
	}
	var filtered []candidate
	for _, c := range cands {
		base := strings.ToLower(filepath.Base(c.name))
		ext := strings.ToLower(filepath.Ext(base))
		if skipBase[base] || skipExt[ext] {
			continue
		}
		l := strings.ToLower(c.name)
		if strings.Contains(l, "completions/") || strings.Contains(l, "man/") {
			continue
		}
		filtered = append(filtered, c)
	}
	if len(filtered) == 0 {
		filtered = cands
	}
	if len(filtered) == 1 {
		return filtered[0].name
	}
	h := strings.ToLower(hint)
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		ai := strings.Contains(strings.ToLower(a.name), h)
		bi := strings.Contains(strings.ToLower(b.name), h)
		if ai != bi {
			return ai
		}
		if a.executable != b.executable {
			return a.executable
		}
		return a.size > b.size
	})
	return filtered[0].name
}

func fromZip(archivePath, destPath, hint string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	var cands []candidate
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		cands = append(cands, candidate{f.Name, int64(f.UncompressedSize64), f.Mode()&0o111 != 0})
	}
	if len(cands) == 0 {
		return errors.New("archive is empty")
	}
	chosen := pickBest(cands, hint)
	for _, f := range r.File {
		if f.Name != chosen {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, rc)
		return err
	}
	return fmt.Errorf("could not locate %q in zip", chosen)
}

func fromTarGz(archivePath, destPath, hint string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var cands []candidate
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		cands = append(cands, candidate{hdr.Name, hdr.Size, hdr.Mode&0o111 != 0})
	}
	if len(cands) == 0 {
		return errors.New("archive is empty")
	}
	chosen := pickBest(cands, hint)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	gz2, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz2.Close()
	tr2 := tar.NewReader(gz2)
	for {
		hdr, err := tr2.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Name != chosen {
			continue
		}
		out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, tr2)
		return err
	}
	return fmt.Errorf("could not locate %q in tar.gz", chosen)
}
