package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const DefaultAPIURL = "https://solutions.slingrs.io/prod/runtime/api"

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
		return applyAPIURLEnv(cfg)
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
	return applyAPIURLEnv(cfg)
}

func applyAPIURLEnv(cfg *Config) (*Config, error) {
	if value := strings.TrimSpace(os.Getenv("PANDA_API_URL")); value != "" {
		validated, err := ValidateAPIURL(value)
		if err != nil {
			return nil, fmt.Errorf("PANDA_API_URL: %w", err)
		}
		cfg.APIURL = validated
	} else {
		validated, err := ValidateAPIURL(cfg.APIURL)
		if err != nil {
			return nil, fmt.Errorf("configured API URL: %w", err)
		}
		cfg.APIURL = validated
	}
	return cfg, nil
}

func ValidateAPIURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("API URL must be an absolute http(s) URL without credentials, query, or fragment")
	}
	return value, nil
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
	return replaceConfigFile(tmpName, path)
}
