package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/config"
	"github.com/espinosajuanma/panda-cli/internal/solutions"
)

func TestConfigCommandSetsAndGetsAPIURL(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PANDA_API_URL", "")
	cfg := &config.Config{APIURL: config.DefaultAPIURL}
	var output strings.Builder
	if err := configCommand(context.Background(), cfg, solutions.NewClient(cfg.APIURL, ""), []string{"set", "api-url", "https://solutions.example/runtime/api"}, &output); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := configCommand(context.Background(), cfg, solutions.NewClient(cfg.APIURL, ""), []string{"get", "api-url"}, &output); err != nil {
		t.Fatal(err)
	}
	if output.String() != "https://solutions.example/runtime/api\n" {
		t.Fatalf("get api-url output = %q", output.String())
	}
}

func TestLogoutClearsSavedSession(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PANDA_API_URL", "")
	cfg := &config.Config{APIURL: config.DefaultAPIURL, Token: "token", Email: "user@example.com"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if err := Run([]string{"logout"}, strings.NewReader(""), &output, &output); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Token != "" || loaded.Email != "" || output.String() != "Logged out.\n" {
		t.Fatalf("logout state=%#v output=%q", loaded, output.String())
	}
}

func TestRunContextPassesCancellationToLoginRequest(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PANDA_API_URL", "https://example.invalid/api")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := RunContext(ctx, []string{"login", "user@example.com"}, strings.NewReader("password\n"), io.Discard, io.Discard)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunContext error = %v, want context cancellation", err)
	}
}
