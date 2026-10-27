package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveUsesPrivateConfigFile(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	cfg := &Config{APIURL: DefaultAPIURL, Token: "private-token", Project: &Project{ID: "p1", Name: "Alpha"}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "panda-dashboard", "config.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions = %o, want 600", info.Mode().Perm())
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Token != cfg.Token || loaded.Project == nil || loaded.Project.ID != "p1" {
		t.Fatalf("config round trip failed: %#v", loaded)
	}
}
