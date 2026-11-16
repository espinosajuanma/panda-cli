package app

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestUnknownCommandSuggestsCloseMatchWithoutPrintingHelp(t *testing.T) {
	setTestConfigHome(t)
	var output, errOutput bytes.Buffer
	err := Run([]string{"pshu"}, strings.NewReader(""), &output, &errOutput)
	if err == nil {
		t.Fatal("unknown command unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), `unknown command "pshu"`) || !strings.Contains(err.Error(), `did you mean "push"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Len() != 0 || errOutput.Len() != 0 {
		t.Fatalf("unknown command printed help: stdout=%q stderr=%q", output.String(), errOutput.String())
	}
}

func TestUnknownCommandWithoutCloseMatchPointsToHelp(t *testing.T) {
	setTestConfigHome(t)
	var output, errOutput bytes.Buffer
	err := Run([]string{"zzzz"}, strings.NewReader(""), &output, &errOutput)
	if err == nil {
		t.Fatal("unknown command unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), `unknown command "zzzz"`) || !strings.Contains(err.Error(), "panda --help") {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Len() != 0 || errOutput.Len() != 0 {
		t.Fatalf("unknown command printed help: stdout=%q stderr=%q", output.String(), errOutput.String())
	}
}

func TestUnknownHelpTopicSuggestsCloseMatch(t *testing.T) {
	var output bytes.Buffer
	err := Run([]string{"help", "pshu"}, strings.NewReader(""), &output, &output)
	if err == nil {
		t.Fatal("unknown help topic unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), `no help available for "pshu"`) || !strings.Contains(err.Error(), `did you mean "push"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("unknown help topic printed help: %q", output.String())
	}
}

func TestUnknownSubcommandsSuggestCloseMatches(t *testing.T) {
	setTestConfigHome(t)
	tests := []struct {
		args []string
		want []string
	}{
		{args: []string{"completion", "bsh"}, want: []string{`unknown completion shell "bsh"`, `did you mean "bash"`}},
		{args: []string{"config", "gte"}, want: []string{`unknown config command "gte"`, `did you mean "get"`}},
		{args: []string{"config", "get", "projct"}, want: []string{`unknown config key "projct"`, `did you mean "project"`}},
	}
	for _, test := range tests {
		t.Run(strings.Join(test.args, "_"), func(t *testing.T) {
			err := Run(test.args, strings.NewReader(""), io.Discard, io.Discard)
			if err == nil {
				t.Fatalf("%v unexpectedly succeeded", test.args)
			}
			for _, want := range test.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestUnknownCatalogAndEditSubcommandsSuggestCloseMatches(t *testing.T) {
	err := projectCatalogCommand(nil, nil, labelCatalog, []string{"craete"}, strings.NewReader(""), io.Discard)
	if err == nil || !strings.Contains(err.Error(), `unknown labels subcommand "craete"`) || !strings.Contains(err.Error(), `did you mean "create"`) {
		t.Fatalf("unexpected catalog error: %v", err)
	}

	err = filesCommandContext(nil, nil, []string{"uplod"}, nil, io.Discard)
	if err == nil || !strings.Contains(err.Error(), `unknown files subcommand "uplod"`) || !strings.Contains(err.Error(), `did you mean "upload"`) {
		t.Fatalf("unexpected files error: %v", err)
	}

	ws, _ := makeSyncedTask(t, map[string]any{
		"id": "task-edit-selector", "number": float64(42), "title": "Edit selector typo",
		"type": "feature", "priority": "medium", "status": "new", "description": "",
	})
	err = editTask(ws, []string{"42", "labls"}, strings.NewReader(""), io.Discard)
	if err == nil || !strings.Contains(err.Error(), `unknown edit selector "labls"`) || !strings.Contains(err.Error(), `did you mean "labels"`) {
		t.Fatalf("unexpected edit error: %v", err)
	}
}
