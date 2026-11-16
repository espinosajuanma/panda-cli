package ui

import (
	"errors"
	"testing"
)

func escapeSequence(bytes ...byte) func() (byte, bool, error) {
	index := 0
	return func() (byte, bool, error) {
		if index >= len(bytes) {
			return 0, false, nil
		}
		value := bytes[index]
		index++
		return value, true, nil
	}
}

func TestDecodeSelectorKeyRecognizesControlKeys(t *testing.T) {
	tests := []struct {
		name  string
		first byte
		rest  []byte
		want  rune
	}{
		{name: "carriage return", first: '\r', want: selectorEnter},
		{name: "newline", first: '\n', want: selectorEnter},
		{name: "space", first: ' ', want: selectorSpace},
		{name: "delete", first: 0x7f, want: selectorBackspace},
		{name: "backspace", first: 0x08, want: selectorBackspace},
		{name: "cancel", first: 0x03, want: selectorCancel},
		{name: "printable", first: 'a', want: 'a'},
		{name: "escape alone", first: 0x1b, want: selectorEscape},
		{name: "escape non sequence", first: 0x1b, rest: []byte{'x'}, want: selectorEscape},
		{name: "up", first: 0x1b, rest: []byte{'[', 'A'}, want: selectorUp},
		{name: "down", first: 0x1b, rest: []byte{'[', 'B'}, want: selectorDown},
		{name: "unknown arrow", first: 0x1b, rest: []byte{'[', 'C'}, want: selectorEscape},
		{name: "incomplete sequence", first: 0x1b, rest: []byte{'['}, want: selectorEscape},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := decodeSelectorKey(test.first, escapeSequence(test.rest...))
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("decodeSelectorKey(%q) = %#v, want %#v", test.first, got, test.want)
			}
		})
	}
}

func TestDecodeSelectorKeyPropagatesEscapeReadErrors(t *testing.T) {
	wantErr := errors.New("read failed")
	_, err := decodeSelectorKey(0x1b, func() (byte, bool, error) {
		return 0, false, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestFilterSelectorOptionsMatchesLabelsCaseInsensitively(t *testing.T) {
	options := []selectorOption{
		{value: "1", label: "Alice"},
		{value: "2", label: "Bob"},
		{value: "3", label: "ALICIA"},
	}
	got := filterSelectorOptions(options, "ali")
	if len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Fatalf("matches = %v, want [0 2]", got)
	}
	if all := filterSelectorOptions(options, ""); len(all) != 3 {
		t.Fatalf("empty filter matches = %v, want all options", all)
	}
}
