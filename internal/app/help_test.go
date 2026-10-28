package app

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestCommandHelpUsesManPageSections(t *testing.T) {
	var output bytes.Buffer
	if err := Run([]string{"help", "edit"}, strings.NewReader(""), &output, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "\x1b[1;32mPANDA-EDIT(1)\x1b[0m") {
		t.Fatalf("man-page title is missing: %q", text)
	}
	for _, section := range []string{"NAME\n", "SYNOPSIS\n", "DESCRIPTION\n", "EXAMPLES\n", "SEE ALSO\n"} {
		if !strings.Contains(text, section) {
			t.Errorf("man-page section %q missing from help: %q", section, text)
		}
	}
	if strings.Contains(text, "Usage:") {
		t.Fatalf("legacy usage format remains in command manual: %q", text)
	}
}

func TestEveryCommandManualHasManPageCoreSections(t *testing.T) {
	for command := range manualPages {
		t.Run(strings.ReplaceAll(command, " ", "_"), func(t *testing.T) {
			var output bytes.Buffer
			if err := printCommandHelp(command, &output); err != nil {
				t.Fatal(err)
			}
			for _, section := range []string{"NAME\n", "SYNOPSIS\n", "DESCRIPTION\n"} {
				if !strings.Contains(output.String(), section) {
					t.Errorf("manual %q missing %q:\n%s", command, section, output.String())
				}
			}
		})
	}
}

func TestNestedCommandHelpResolvesToSubcommandManual(t *testing.T) {
	tests := []struct {
		args     []string
		page     string
		synopsis string
	}{
		{args: []string{"help", "config", "set", "project"}, page: "PANDA-CONFIG-SET-PROJECT(1)", synopsis: "panda config set project <name>"},
		{args: []string{"config", "set", "project", "--help"}, page: "PANDA-CONFIG-SET-PROJECT(1)", synopsis: "panda config set project <name>"},
		{args: []string{"completion", "labels", "--help"}, page: "PANDA-COMPLETION-LABELS(1)", synopsis: "panda completion labels [prefix]"},
		{args: []string{"completion", "bash", "--help"}, page: "PANDA-COMPLETION-BASH(1)", synopsis: "panda completion bash"},
	}
	for _, test := range tests {
		var output bytes.Buffer
		if err := Run(test.args, strings.NewReader(""), &output, &output); err != nil {
			t.Fatalf("Run(%v): %v", test.args, err)
		}
		if !strings.Contains(output.String(), test.page) || !strings.Contains(output.String(), test.synopsis) {
			t.Fatalf("Run(%v) did not print the selected subcommand manual:\n%s", test.args, output.String())
		}
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

func TestGlobalHelpHasManSectionsAndCommandOptions(t *testing.T) {
	var output bytes.Buffer
	if err := Run([]string{"--help"}, strings.NewReader(""), &output, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "\x1b[1;32mPANDA(1)\x1b[0m") {
		t.Fatalf("main manual title is missing: %q", text)
	}
	for _, expected := range []string{
		"NAME\n", "SYNOPSIS\n", "DESCRIPTION\n", "COMMANDS\n", "GLOBAL OPTIONS\n",
		"--current", "--staging", "--released", "--discarded", "--all", "--status",
		"--no-complexity", "--no-description", "--verbose", "--summary", "--yes",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("main help missing %q:\n%s", expected, text)
		}
	}
}
