package app

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestReadSelectorKeyRecognizesNavigationAndSelectionKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  rune
	}{
		{name: "up", input: "\x1b[A", want: rune(selectorUp)},
		{name: "down", input: "\x1b[B", want: rune(selectorDown)},
		{name: "enter", input: "\r", want: rune(selectorEnter)},
		{name: "space", input: " ", want: rune(selectorSpace)},
		{name: "cancel", input: "\x03", want: rune(selectorCancel)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			if _, err := writer.Write([]byte(test.input)); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			got, err := readSelectorKey(reader)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("readSelectorKey() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestRenderSelectorReturnsToColumnZeroOnEveryLine(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	options := []selectorOption{{value: "feature", label: "feature"}, {value: "bug", label: "bug"}}
	renderSelector(writer, "Type", options, []int{0, 1}, 1, "", nil, false, 0)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	for index, value := range data {
		if value == '\n' && (index == 0 || data[index-1] != '\r') {
			t.Fatalf("selector output has a bare line feed at byte %d: %q", index, data)
		}
	}
}

func TestFilterSelectorOptionsMatchesTypedSearch(t *testing.T) {
	options := []selectorOption{{value: "", label: "None (clear all)"}, {value: "bug", label: "Bug"}, {value: "security", label: "Security"}}
	if got, want := filterSelectorOptions(options, "sec"), []int{2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("filterSelectorOptions() = %v, want %v", got, want)
	}
	if got, want := filterSelectorOptions(options, ""), []int{0, 1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unfiltered options = %v, want %v", got, want)
	}
}

func TestDescriptionEditorUsesSingleEKeyOnly(t *testing.T) {
	if !descriptionEditorRequested('e') || !descriptionEditorRequested('E') {
		t.Fatal("e should launch the description editor")
	}
	if descriptionEditorRequested(rune(selectorEnter)) || descriptionEditorRequested('n') {
		t.Fatal("Enter and other keys should skip the description editor")
	}
}

func TestDescriptionPromptUsesRequestedLabel(t *testing.T) {
	var output strings.Builder
	if err := promptDescriptionEditor(strings.NewReader("\n"), &output, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Description [(e) to launch editor, enter to skip]") {
		t.Fatalf("description prompt = %q", output.String())
	}
}
