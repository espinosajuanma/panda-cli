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
	plain := ansiEscapeSequence.ReplaceAllString(text, "")
	for _, section := range []string{"NAME", "SYNOPSIS", "DESCRIPTION", "ARGUMENTS", "EXAMPLES", "SEE ALSO"} {
		if !strings.Contains(plain, section+"\n") || !strings.Contains(text, "\x1b[1;32m"+section+"\x1b[0m") {
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
			text := output.String()
			plain := ansiEscapeSequence.ReplaceAllString(text, "")
			for _, section := range []string{"NAME", "SYNOPSIS", "DESCRIPTION"} {
				if !strings.Contains(plain, section+"\n") || !strings.Contains(text, "\x1b[1;32m"+section+"\x1b[0m") {
					t.Errorf("manual %q missing %q:\n%s", command, section, output.String())
				}
			}
		})
	}
}

func TestOptionSectionTitlesAreBoldGreen(t *testing.T) {
	var output bytes.Buffer
	if err := printCommandHelp("list", &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "\x1b[1;32mOPTIONS\x1b[0m") {
		t.Fatalf("OPTIONS heading is not bold green:\n%s", text)
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
		{args: []string{"labels", "edit", "--help"}, page: "PANDA-LABELS-EDIT(1)", synopsis: "panda labels edit <name|id>"},
		{args: []string{"completion", "bash", "--help"}, page: "PANDA-COMPLETION-BASH(1)", synopsis: "panda completion bash"},
		{args: []string{"labels", "create", "--help"}, page: "PANDA-LABELS-CREATE(1)", synopsis: "panda labels create [name] [description]"},
		{args: []string{"packages", "delete", "--help"}, page: "PANDA-PACKAGES-DELETE(1)", synopsis: "panda packages delete <title|id>"},
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

func TestCompletionNoLongerProvidesLabelSuggestions(t *testing.T) {
	var output bytes.Buffer
	err := Run([]string{"completion", "labels"}, strings.NewReader(""), &output, &output)
	if err == nil || !strings.Contains(err.Error(), "usage: panda completion <bash|zsh|fish>") {
		t.Fatalf("completion labels should be unavailable, got output=%q error=%v", output.String(), err)
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
	plain := ansiEscapeSequence.ReplaceAllString(text, "")
	for _, expected := range []string{
		"NAME\n", "SYNOPSIS\n", "DESCRIPTION\n", "COMMANDS\n", "GLOBAL OPTIONS\n", "ENVIRONMENT\n", "FILES\n",
		"--current", "--staging", "--released", "--discarded", "--all", "--status",
		"--no-complexity", "--no-description", "--no-package", "--no-labels",
		"--verbose", "-v", "--summary", "-s", "--yes", "panda labels", "panda packages",
	} {
		if !strings.Contains(plain, expected) {
			t.Errorf("main help missing %q:\n%s", expected, text)
		}
	}
	for _, section := range []string{"NAME", "SYNOPSIS", "DESCRIPTION", "COMMANDS", "GLOBAL OPTIONS", "ENVIRONMENT", "FILES"} {
		if !strings.Contains(text, "\x1b[1;32m"+section+"\x1b[0m") {
			t.Errorf("main help section %q is not bold green:\n%s", section, text)
		}
	}
}

func TestHelpOutputPagingThreshold(t *testing.T) {
	small := []byte("short manual\n")
	large := []byte(strings.Repeat("long manual line\n", 30))
	if shouldPageHelp(false, large, 80, 24) {
		t.Fatal("non-terminal help output should not use a pager")
	}
	if shouldPageHelp(true, small, 80, 24) {
		t.Fatal("help that fits the terminal should not use a pager")
	}
	if !shouldPageHelp(true, large, 80, 24) {
		t.Fatal("help longer than the terminal should use a pager")
	}
}
