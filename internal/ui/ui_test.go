package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
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
	if runtime.GOOS == "windows" {
		t.Skip("editor fixture requires a POSIX shell")
	}
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

func TestSelectorFrameTruncatesToAvailableWidthAndExpandsWithWindow(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "selector-output")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	options := []selectorOption{{value: "task-1", label: "A very long task title that must fit on one terminal row"}}
	narrow := buildSelectorFrame(file, "Tasks · type to filter", options, []int{0}, 0, strings.Repeat("filter", 12), nil, false, 32, 12)
	if rows := selectorRowsForWidth(narrow, 32); rows != len(narrow) {
		t.Fatalf("narrow selector frame uses %d rows, want %d: %#v", rows, len(narrow), narrow)
	}
	for _, line := range narrow {
		if width := selectorDisplayWidth(line); width > 31 {
			t.Errorf("selector line width %d exceeds available 31 columns: %q", width, line)
		}
	}
	if !strings.HasSuffix(narrow[1], "ilter") {
		t.Fatalf("narrow filter did not retain its suffix: %q", narrow[1])
	}

	wide := buildSelectorFrame(file, "Tasks · type to filter", options, []int{0}, 0, "", nil, false, 80, 24)
	if !strings.Contains(wide[1], options[0].label) {
		t.Fatalf("wide selector unexpectedly truncated task label: %#v", wide)
	}
	if selectorRowsForWidth(wide, 32) <= len(wide) {
		t.Fatalf("selector frame from a wider window did not account for resize reflow: %#v", wide)
	}
}

func TestSelectorRowsIgnoreANSIAndCountWrapping(t *testing.T) {
	lines := []string{"\x1b[1;36m1234567890123456789012345\x1b[0m", "short"}
	if got := selectorRowsForWidth(lines, 10); got != 4 {
		t.Fatalf("wrapped selector row count = %d, want 4", got)
	}
}
