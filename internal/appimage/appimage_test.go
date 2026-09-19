package appimage

import (
	"testing"

	"github.com/rokiri/clap/internal/ghrelease"
)

func TestIsAppImageAsset(t *testing.T) {
	cases := map[string]bool{
		"myapp-x86_64.AppImage": true,
		"myapp.appimage":        true,
		"myapp.AppImage.zsync":  false,
		"myapp.tar.gz":          false,
		"myapp":                 false,
	}
	for name, want := range cases {
		if got := IsAppImageAsset(name); got != want {
			t.Errorf("IsAppImageAsset(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestFindAssetPrefersHostArch(t *testing.T) {
	rel := &ghrelease.Release{
		TagName: "v1.0.0",
		Assets: []ghrelease.Asset{
			{Name: "myapp-i686.AppImage"},
			{Name: "myapp-x86_64.AppImage"},
			{Name: "myapp-aarch64.AppImage"},
			{Name: "myapp.tar.gz"},
			{Name: "checksums.txt"},
		},
	}
	a := FindAsset(rel)
	if a == nil {
		t.Fatal("expected a match, got nil")
	}
	if !IsAppImageAsset(a.Name) {
		t.Fatalf("FindAsset picked a non-AppImage asset: %s", a.Name)
	}
}

func TestFindAssetFallsBackToGenericAppImage(t *testing.T) {
	rel := &ghrelease.Release{
		Assets: []ghrelease.Asset{
			{Name: "myapp.AppImage"},
			{Name: "myapp.tar.gz"},
		},
	}
	a := FindAsset(rel)
	if a == nil || a.Name != "myapp.AppImage" {
		t.Fatalf("expected fallback to the only AppImage asset, got %+v", a)
	}
}

func TestFindAssetNoMatch(t *testing.T) {
	rel := &ghrelease.Release{
		Assets: []ghrelease.Asset{
			{Name: "myapp.tar.gz"},
			{Name: "myapp.deb"},
		},
	}
	if a := FindAsset(rel); a != nil {
		t.Fatalf("expected no match, got %+v", a)
	}
}
