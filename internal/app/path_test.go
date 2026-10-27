package app

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"panda-dashboard/internal/config"
	"panda-dashboard/internal/workspace"
)

func TestPathPrintsActiveWorkspacePathWithoutCreatingIt(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	if err := (&config.Config{
		APIURL:  config.DefaultAPIURL,
		Project: &config.Project{ID: "project-1", Name: "Stokes Wagner"},
	}).Save(); err != nil {
		t.Fatal(err)
	}

	expected, err := workspace.WorkspacePath("Stokes Wagner")
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Run([]string{"path"}, strings.NewReader(""), &output, &output); err != nil {
		t.Fatal(err)
	}
	if output.String() != expected+"\n" {
		t.Fatalf("path output = %q, want %q", output.String(), expected+"\n")
	}
	if _, err := os.Stat(expected); !os.IsNotExist(err) {
		t.Fatalf("path command unexpectedly created workspace: %v", err)
	}
}
