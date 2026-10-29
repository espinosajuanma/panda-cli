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

func TestProjectViewIDDefaultsAndCanBeConfigured(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &Config{Project: &Project{ID: "p1", Name: "Alpha"}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Project == nil || loaded.Project.ViewID != DefaultProjectViewID {
		t.Fatalf("legacy project config view ID = %#v, want default %q", loaded.Project, DefaultProjectViewID)
	}

	loaded.Project.ViewID = "custom-view-id"
	if err := loaded.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Project.ViewID != "custom-view-id" {
		t.Fatalf("configured project view ID = %q, want custom-view-id", loaded.Project.ViewID)
	}
}
