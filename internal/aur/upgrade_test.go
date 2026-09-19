package aur

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUpgradeGracefulWithoutVercmp(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("CLAP_HOME", tmpHome)

	dbDir := filepath.Join(tmpHome, "aur")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seed := DB{Entries: map[string]DBEntry{
		"mypkg": {Name: "mypkg", PackageBase: "mypkg", Version: "1.0.0-1"},
	}}
	data, _ := json.Marshal(seed)
	if err := os.WriteFile(filepath.Join(dbDir, "installed.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc/v5/info", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(rpcResponse{
			Version: 5, Type: "multiinfo", ResultCount: 1,
			Results: []Package{{Name: "mypkg", PackageBase: "mypkg", Version: "2.0.0-1"}},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	old := rpcBase
	rpcBase = srv.URL + "/rpc/v5"
	defer func() { rpcBase = old }()

	if err := Upgrade(); err != nil {
		t.Fatalf("Upgrade should not hard-fail when vercmp is unavailable, got: %v", err)
	}
}
