package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const DefaultAPIURL = "https://solutions.slingrs.io/prod/runtime/api"
const DefaultProjectViewID = "680b8e41805d3740e97cfecb"

type Project struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	ViewID string `json:"view_id,omitempty"`
}

type Config struct {
	APIURL  string   `json:"api_url,omitempty"`
	Token   string   `json:"token,omitempty"`
	Email   string   `json:"email,omitempty"`
	Project *Project `json:"project,omitempty"`
}

func Directory() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "panda-dashboard"), nil
}

func WorkspaceDirectory() (string, error) {
	dir, err := Directory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "workspaces"), nil
}

func Load() (*Config, error) {
	dir, err := Directory()
	if err != nil {
		return nil, err
	}
	cfg := &Config{APIURL: DefaultAPIURL}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.APIURL == "" {
		cfg.APIURL = DefaultAPIURL
	}
	if cfg.Project != nil && cfg.Project.ViewID == "" {
		cfg.Project.ViewID = DefaultProjectViewID
	}
	return cfg, nil
}

func (c *Config) Save() error {
	dir, err := Directory()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "config.json")
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
