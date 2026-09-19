package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rokiri/clap/internal/archivex"
	"github.com/rokiri/clap/internal/builder"
	"github.com/rokiri/clap/internal/ghrelease"
	"github.com/rokiri/clap/internal/manifest"
	"github.com/rokiri/clap/internal/pkgfile"
	"github.com/rokiri/clap/internal/store"
)

type Options struct {
	URL        string
	Name       string
	Tag        string
	AssetGlob  string
	OS         string
	Arch       string
	SkipVerify bool
	Lib        bool
}

type Result struct {
	Name string
	Tag  string
	Kind string
}

func Install(o Options) (*Result, error) {
	owner, repo, err := builder.ParseGitHubURL(o.URL)
	if err != nil {
		return nil, err
	}
	name := o.Name
	if name == "" {
		name = repo
	}
	targetOS := o.OS
	if targetOS == "" {
		targetOS = runtime.GOOS
	}
	targetArch := o.Arch
	if targetArch == "" {
		targetArch = runtime.GOARCH
	}
	kind := "app"
	if o.Lib {
		kind = "library"
	}
	ext := manifest.ExtFor(kind)

	if err := os.MkdirAll(store.AppsDir(), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(store.TmpDir(), 0o755); err != nil {
		return nil, err
	}

	fmt.Printf("clap: resolving release for %s/%s...\n", owner, repo)
	rel, asset, err := ghrelease.FindPackageAsset(owner, repo, o.Tag, o.AssetGlob, ext)
	if err == nil && asset != nil {
		fmt.Printf("clap: found prebuilt %s asset %q in release %s\n", ext, asset.Name, rel.TagName)
		dest := filepath.Join(store.AppsDir(), name+ext)
		if err := ghrelease.Download(asset.BrowserDownloadURL, dest); err != nil {
			return nil, fmt.Errorf("downloading asset: %w", err)
		}
		fmt.Printf("clap: installed %q -> %s\n", name, dest)
		return &Result{Name: name, Tag: rel.TagName, Kind: kind}, nil
	}

	if rel != nil {
		if binAsset := ghrelease.FindBinaryAsset(rel, targetOS, targetArch); binAsset != nil {
			fmt.Printf("clap: found matching asset %q in release %s\n", binAsset.Name, rel.TagName)
			downloadPath := filepath.Join(store.TmpDir(), name+"-dl-"+binAsset.Name)
			if err := ghrelease.Download(binAsset.BrowserDownloadURL, downloadPath); err != nil {
				return nil, fmt.Errorf("downloading asset: %w", err)
			}
			if !o.SkipVerify {
				if cs := ghrelease.FindChecksumAsset(rel); cs != nil {
					ok, err := ghrelease.VerifyChecksum(cs.BrowserDownloadURL, binAsset.Name, downloadPath)
					switch {
					case err != nil:
						return nil, fmt.Errorf("checksum verification failed: %w", err)
					case ok:
						fmt.Println("clap: checksum verified OK")
					default:
						fmt.Println("clap: no checksum entry found for this asset, skipping verification")
					}
				}
			}
			binPath := downloadPath
			lower := strings.ToLower(binAsset.Name)
			if strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
				fmt.Println("clap: unpacking archive...")
				extracted := filepath.Join(store.TmpDir(), name+"-bin")
				if err := archivex.ExtractBinary(downloadPath, extracted, name); err != nil {
					return nil, fmt.Errorf("extracting archive: %w", err)
				}
				binPath = extracted
			}
			_ = os.Chmod(binPath, 0o755)
			m := manifest.Manifest{
				Name: name, Repo: owner + "/" + repo, Tag: rel.TagName,
				Entry: name, OS: targetOS, Arch: targetArch,
				Source: "release-binary", Kind: kind, ClapVer: manifest.Version,
			}
			dest := filepath.Join(store.AppsDir(), name+ext)
			if err := pkgfile.Package(dest, binPath, name, m); err != nil {
				return nil, err
			}
			fmt.Printf("clap: installed %q -> %s\n", name, dest)
			return &Result{Name: name, Tag: rel.TagName, Kind: kind}, nil
		}
	}

	if targetOS != runtime.GOOS || targetArch != runtime.GOARCH {
		return nil, fmt.Errorf("no release asset found for %s/%s, and cross-building from source isn't supported", targetOS, targetArch)
	}

	if o.Lib {
		fmt.Println("clap: no usable release asset found, cloning and vendoring source...")
		if err := builder.VendorLibrary(owner, repo, name, o.Tag); err != nil {
			return nil, err
		}
		return &Result{Name: name, Tag: o.Tag, Kind: kind}, nil
	}

	fmt.Println("clap: no usable release asset found, cloning and building from source...")
	if err := builder.FromSource(owner, repo, name, o.Tag); err != nil {
		return nil, err
	}
	return &Result{Name: name, Tag: o.Tag, Kind: kind}, nil
}
