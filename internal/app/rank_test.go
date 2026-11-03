package app

import (
	"io"
	"strings"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func TestEditRankSwapsWithAdjacentTaskInSameStatus(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []map[string]any{
		listTask("rank-a", 101, "new", "a", nil, "First", "feature"),
		listTask("rank-b", 102, "new", "b", nil, "Selected", "feature"),
		listTask("rank-c", 103, "new", "c", nil, "Last", "feature"),
		listTask("rank-todo", 104, "toDo", "a", nil, "Other status", "feature"),
	} {
		addSyncedTask(t, ws, fixture)
	}

	var output strings.Builder
	if err := editTask(ws, []string{"102", "rank", "up"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	firstDir, firstMetadata, err := ws.FindTask("101")
	if err != nil {
		t.Fatal(err)
	}
	selectedDir, selected, err := ws.FindTask("102")
	if err != nil {
		t.Fatal(err)
	}
	if stringField(selected, "rank") != "a" {
		t.Fatalf("selected task rank = %q, want a", stringField(selected, "rank"))
	}
	if stringField(firstMetadata, "rank") != "b" {
		t.Fatalf("adjacent task rank = %q, want b", stringField(firstMetadata, "rank"))
	}
	otherDir, otherStatusTask, err := ws.FindTask("104")
	if err != nil {
		t.Fatal(err)
	}
	if stringField(otherStatusTask, "rank") != "a" {
		t.Fatalf("task in another status changed rank: %q", stringField(otherStatusTask, "rank"))
	}
	if !strings.Contains(output.String(), "publish both changes") {
		t.Fatalf("rank move did not explain push behavior: %q", output.String())
	}
	if selectedDir == "" || firstDir == "" || otherDir == "" {
		t.Fatal("task directory was empty")
	}
	if err := editTask(ws, []string{"102", "rank", "down"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	_, selected, err = ws.FindTask("102")
	if err != nil {
		t.Fatal(err)
	}
	_, firstMetadata, err = ws.FindTask("101")
	if err != nil {
		t.Fatal(err)
	}
	if stringField(selected, "rank") != "b" || stringField(firstMetadata, "rank") != "a" {
		t.Fatalf("down move did not swap adjacent ranks: selected=%q neighbor=%q", stringField(selected, "rank"), stringField(firstMetadata, "rank"))
	}
}

func TestEditRankRejectsBoundaryAndMissingRanks(t *testing.T) {
	tests := []struct {
		name      string
		first     map[string]any
		second    map[string]any
		selectID  string
		direction string
		wantErr   string
	}{
		{
			name: "already first", first: listTask("first", 201, "new", "a", nil, "First", "feature"),
			second: listTask("second", 202, "new", "b", nil, "Second", "feature"), selectID: "201", direction: "up", wantErr: "already first",
		},
		{
			name: "already last", first: listTask("first", 201, "new", "a", nil, "First", "feature"),
			second: listTask("second", 202, "new", "b", nil, "Second", "feature"), selectID: "202", direction: "down", wantErr: "already last",
		},
		{
			name: "missing selected rank", first: listTask("first", 201, "new", "a", nil, "First", "feature"),
			second: listTask("second", 202, "new", "", nil, "Second", "feature"), selectID: "202", direction: "up", wantErr: "without fetched ranks",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ws := workspace.New(t.TempDir())
			if err := ws.Ensure(); err != nil {
				t.Fatal(err)
			}
			addSyncedTask(t, ws, test.first)
			addSyncedTask(t, ws, test.second)
			if err := editTask(ws, []string{test.selectID, "rank", test.direction}, strings.NewReader(""), io.Discard); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("editTask error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestEditRankRejectsInvalidDirection(t *testing.T) {
	ws := workspace.New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	addSyncedTask(t, ws, listTask("rank-one", 301, "new", "a", nil, "One", "feature"))
	if err := editTask(ws, []string{"301", "rank", "sideways"}, strings.NewReader(""), io.Discard); err == nil || !strings.Contains(err.Error(), "choose up or down") {
		t.Fatalf("invalid direction error = %v", err)
	}
}
