package main

import (
	"bytes"
	"strings"
	"testing"
)

func setTestConfigHome(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("HOME", base)
	t.Setenv("AppData", base)
}

func TestRunHelpExitsZero(t *testing.T) {
	setTestConfigHome(t)
	var output, errOutput bytes.Buffer
	code := run([]string{"--help"}, strings.NewReader(""), &output, &errOutput)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, errOutput.String())
	}
	if !strings.Contains(output.String(), "panda") || errOutput.Len() != 0 {
		t.Fatalf("help output=%q stderr=%q", output.String(), errOutput.String())
	}
}

func TestRunUnknownCommandExitsOne(t *testing.T) {
	setTestConfigHome(t)
	var output, errOutput bytes.Buffer
	code := run([]string{"not-a-command"}, strings.NewReader(""), &output, &errOutput)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOutput.String(), "panda:") || !strings.Contains(errOutput.String(), "unknown command") {
		t.Fatalf("stderr = %q", errOutput.String())
	}
}
