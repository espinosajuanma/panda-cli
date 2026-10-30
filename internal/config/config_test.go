package config

import (
	"os"
	"path/filepath"
	"strings"
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
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("config directory permissions = %o, want 700", dirInfo.Mode().Perm())
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Token != cfg.Token || loaded.Project == nil || loaded.Project.ID != "p1" {
		t.Fatalf("config round trip failed: %#v", loaded)
	}
}

func TestLoadIgnoresLegacyProjectViewID(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	dir := filepath.Join(base, "panda-dashboard")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"project":{"id":"p1","name":"Alpha","view_id":"old-custom-view"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Project == nil || loaded.Project.ID != "p1" || loaded.Project.Name != "Alpha" {
		t.Fatalf("legacy project config did not load: %#v", loaded.Project)
	}
	if err := loaded.Save(); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), "view_id") {
		t.Fatalf("obsolete per-project view setting was saved: %s", saved)
	}
}

func TestLoadUsesValidatedAPIURLEnvironmentOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PANDA_API_URL", "https://solutions.example/runtime/api/")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIURL != "https://solutions.example/runtime/api" {
		t.Fatalf("API URL = %q", cfg.APIURL)
	}
	t.Setenv("PANDA_API_URL", "file:///etc/passwd")
	if _, err := Load(); err == nil {
		t.Fatal("invalid API URL override was accepted")
	}
}
