package app

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestCommandHelpHasUppercaseBoldGreenTitle(t *testing.T) {
	var output bytes.Buffer
	if err := Run([]string{"help", "edit"}, strings.NewReader(""), &output, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "\x1b[1;32mEDIT\x1b[0m") {
		t.Fatalf("help title is not bold green uppercase: %q", output.String())
	}
	if !strings.Contains(output.String(), "Usage: panda edit") {
		t.Fatalf("usage missing from help: %q", output.String())
	}
}

func TestBashCompletionScriptIsValid(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var output bytes.Buffer
	if err := Run([]string{"completion", "bash"}, strings.NewReader(""), &output, &output); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("bash", "-n")
	command.Stdin = strings.NewReader(output.String())
	if result, err := command.CombinedOutput(); err != nil {
		t.Fatalf("invalid bash completion script: %v\n%s", err, result)
	}
}

func TestGlobalHelpHasUppercaseBoldGreenTitle(t *testing.T) {
	var output bytes.Buffer
	if err := Run([]string{"--help"}, strings.NewReader(""), &output, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "\x1b[1;32mPANDA DASHBOARD\x1b[0m") {
		t.Fatalf("global help title is not bold green uppercase: %q", output.String())
	}
}
