package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSplitCommandPreservesQuotedArguments(t *testing.T) {
	words, err := SplitCommand(`"/path with spaces/editor" --wait 'single quoted' escaped\ word`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/path with spaces/editor", "--wait", "single quoted", "escaped word"}
	if !reflect.DeepEqual(words, want) {
		t.Fatalf("SplitCommand() = %#v, want %#v", words, want)
	}
}

func TestSplitCommandPreservesWindowsBackslashesInsideQuotedPath(t *testing.T) {
	words, err := SplitCommand(`"C:\Program Files\Editor\editor.exe" --wait`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`C:\Program Files\Editor\editor.exe`, "--wait"}
	if !reflect.DeepEqual(words, want) {
		t.Fatalf("SplitCommand() = %#v, want %#v", words, want)
	}
}

func TestSplitCommandRejectsIncompleteSyntax(t *testing.T) {
	for _, input := range []string{`"unterminated`, `trailing\`} {
		if _, err := SplitCommand(input); err == nil {
			t.Errorf("SplitCommand(%q) accepted incomplete syntax", input)
		}
	}
}

func TestPromptReadsLineAndTrimsWhitespace(t *testing.T) {
	var output bytes.Buffer
	got, err := Prompt(strings.NewReader("  panda  \nignored"), &output, "Name: ")
	if err != nil || got != "panda" || output.String() != "? Name: " {
		t.Fatalf("Prompt() = %q, %v, output=%q", got, err, output.String())
	}
}

func TestPageOutputFallsBackWhenPagerCommandIsInvalid(t *testing.T) {
	t.Setenv("PAGER", `"unterminated`)
	var output bytes.Buffer
	if err := PageOutput(&output, []byte("content")); err != nil {
		t.Fatal(err)
	}
	if output.String() != "content" {
		t.Fatalf("fallback output = %q", output.String())
	}
}

func TestConfigureLessPagerAddsRawAndNoInitOptions(t *testing.T) {
	got := ConfigureLessPager([]string{"/usr/bin/less", "-F"})
	want := []string{"/usr/bin/less", "-F", "-R", "-X"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ConfigureLessPager() = %#v, want %#v", got, want)
	}
}

func TestOpenEditorParsesQuotedCommandPath(t *testing.T) {
	dir := t.TempDir()
	commandPath := filepath.Join(dir, "editor command")
	marker := filepath.Join(dir, "marker file")
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nprintf '%s' \"$2\" > \""+marker+"\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(dir, "target file.md")
	if err := os.WriteFile(filePath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", `"`+commandPath+`" --wait`)
	if err := OpenEditor(filePath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != filePath {
		t.Fatalf("editor argument = %q, error=%v", data, err)
	}
}
