package archpkg

import (
	"testing"

	"github.com/rokiri/clap/internal/ghrelease"
)

func TestIsPackageAsset(t *testing.T) {
	cases := map[string]bool{
		"myapp-1.0.0-1-x86_64.pkg.tar.zst": true,
		"myapp-1.0.0-1-any.pkg.tar.xz":     true,
		"myapp.tar.gz":                     false,
		"myapp.AppImage":                   false,
		"myapp.pkg.tar.zst.sig":            false,
	}
	for name, want := range cases {
		if got := IsPackageAsset(name); got != want {
			t.Errorf("IsPackageAsset(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestFindAssetExactArch(t *testing.T) {
	rel := &ghrelease.Release{
		Assets: []ghrelease.Asset{
			{Name: "myapp-1.0.0-1-aarch64.pkg.tar.zst"},
			{Name: "myapp-1.0.0-1-x86_64.pkg.tar.zst"},
			{Name: "myapp-1.0.0-1-any.pkg.tar.zst"},
		},
	}
	a := FindAsset(rel)
	if a == nil {
		t.Fatal("expected a match")
	}
	if a.Name != "myapp-1.0.0-1-"+pacmanArch()+".pkg.tar.zst" {
		t.Fatalf("expected exact-arch match for %s, got %s", pacmanArch(), a.Name)
	}
}

func TestFindAssetFallsBackToAny(t *testing.T) {
	rel := &ghrelease.Release{
		Assets: []ghrelease.Asset{
			{Name: "myapp-1.0.0-1-any.pkg.tar.zst"},
		},
	}
	a := FindAsset(rel)
	if a == nil || a.Name != "myapp-1.0.0-1-any.pkg.tar.zst" {
		t.Fatalf("expected fallback to -any package, got %+v", a)
	}
}

func TestFindAssetNoMatch(t *testing.T) {
	rel := &ghrelease.Release{
		Assets: []ghrelease.Asset{
			{Name: "myapp.deb"},
			{Name: "myapp.tar.gz"},
		},
	}
	if a := FindAsset(rel); a != nil {
		t.Fatalf("expected no match, got %+v", a)
	}
}
