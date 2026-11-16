package app

import "testing"

func TestClosestMatchSuggestsLikelyCommandTypos(t *testing.T) {
	commands := []string{"push", "pull", "list", "login", "completion", "help", "config", "labels", "create", "bash", "zsh", "get", "project"}
	tests := []struct {
		input string
		want  string
	}{
		{input: "pshu", want: "push"},
		{input: "hepl", want: "help"},
		{input: "lst", want: "list"},
		{input: "pul", want: "pull"},
		{input: "labls", want: "labels"},
		{input: "craete", want: "create"},
		{input: "bsh", want: "bash"},
		{input: "gte", want: "get"},
		{input: "projct", want: "project"},
		{input: "comp", want: "completion"},
		{input: "PSHU", want: "push"},
		{input: "zzzz", want: ""},
		{input: "unrelated", want: ""},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			if got := closestMatch(test.input, commands); got != test.want {
				t.Fatalf("closestMatch(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}
