package aur

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mockAURServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/rpc/v5/search/", func(w http.ResponseWriter, r *http.Request) {
		resp := rpcResponse{
			Version: 5, Type: "search", ResultCount: 2,
			Results: []Package{
				{Name: "yay", Version: "12.3.5-1", Description: "Yet another yogurt. Pacman wrapper and AUR helper written in go."},
				{Name: "yay-bin", Version: "12.3.5-1", Description: "Yet another yogurt. Pacman wrapper and AUR helper written in go (prebuilt)."},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/rpc/v5/info", func(w http.ResponseWriter, r *http.Request) {
		args := r.URL.Query()["arg[]"]
		var results []Package
		for _, a := range args {
			switch a {
			case "mypkg":
				results = append(results, Package{
					Name: "mypkg", PackageBase: "mypkg", Version: "1.0.0-1",
					Depends:     []string{"glibc", "some-aur-only-lib>=2.0"},
					MakeDepends: []string{"cmake"},
				})
			case "some-aur-only-lib":
				results = append(results, Package{
					Name: "some-aur-only-lib", PackageBase: "some-aur-only-lib", Version: "2.1.0-1",
				})
			case "missing-pkg":
			}
		}
		json.NewEncoder(w).Encode(rpcResponse{Version: 5, Type: "multiinfo", ResultCount: len(results), Results: results})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSearch(t *testing.T) {
	srv := mockAURServer(t)
	old := rpcBase
	rpcBase = srv.URL + "/rpc/v5"
	defer func() { rpcBase = old }()

	pkgs, err := Search("yay", "")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 results, got %d", len(pkgs))
	}
	if pkgs[0].Name != "yay" || pkgs[1].Name != "yay-bin" {
		t.Fatalf("unexpected results: %+v", pkgs)
	}
}

func TestInfoOne(t *testing.T) {
	srv := mockAURServer(t)
	old := rpcBase
	rpcBase = srv.URL + "/rpc/v5"
	defer func() { rpcBase = old }()

	pkg, err := InfoOne("mypkg")
	if err != nil {
		t.Fatalf("InfoOne returned error: %v", err)
	}
	if pkg.Version != "1.0.0-1" {
		t.Fatalf("unexpected version: %s", pkg.Version)
	}
	if len(pkg.Depends) != 2 {
		t.Fatalf("expected 2 depends, got %d: %v", len(pkg.Depends), pkg.Depends)
	}
}

func TestInfoOneNotFound(t *testing.T) {
	srv := mockAURServer(t)
	old := rpcBase
	rpcBase = srv.URL + "/rpc/v5"
	defer func() { rpcBase = old }()

	_, err := InfoOne("missing-pkg")
	if err == nil {
		t.Fatal("expected error for missing package, got nil")
	}
}

func TestStripVersionConstraint(t *testing.T) {
	cases := map[string]string{
		"glibc":            "glibc",
		"glibc>=2.31":      "glibc",
		"glibc<=2.31":      "glibc",
		"foo=1.0":          "foo",
		"bar":              "bar",
		"some-aur-lib>2.0": "some-aur-lib",
	}
	for in, want := range cases {
		got := StripVersionConstraint(in)
		if got != want {
			t.Errorf("StripVersionConstraint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParsePackageName(t *testing.T) {
	cases := map[string]string{
		"vercel": "vercel",
		"https://aur.archlinux.org/packages/vercel":      "vercel",
		"http://aur.archlinux.org/packages/vercel":       "vercel",
		"aur.archlinux.org/packages/vercel":              "vercel",
		"https://aur.archlinux.org/packages/vercel/":     "vercel",
		"https://aur.archlinux.org/pkgbase/vercel":       "vercel",
		"https://aur.archlinux.org/packages/yay-bin?a=1": "yay-bin",
		"https://aur.archlinux.org/packages/yay-bin#top": "yay-bin",
		"  yay-bin  ": "yay-bin",
	}
	for in, want := range cases {
		got := ParsePackageName(in)
		if got != want {
			t.Errorf("ParsePackageName(%q) = %q, want %q", in, got, want)
		}
	}
}
