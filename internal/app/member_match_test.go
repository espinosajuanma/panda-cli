package app

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

func TestNormalizedProjectMemberMatching(t *testing.T) {
	members := []map[string]any{
		{"user": map[string]any{"id": "juanma-id", "label": "Juanma Espinosa", "email": "juanma@example.com"}, "role": "developer"},
		{"user": map[string]any{"id": "juan-perez-id", "label": "Juan Pérez", "email": "juan@example.com"}, "role": "qa"},
		{"user": map[string]any{"id": "jose-id", "label": "José Alvarez", "email": "jose@example.com"}, "role": "teamLead"},
	}
	if got := normalizeMemberSelector(" Juan-Ma "); got != "juanma" {
		t.Fatalf("normalized selector = %q, want juanma", got)
	}
	match, err := resolveAssignableProjectMember(members, members, "juanma")
	if err != nil || relationshipID(memberUser(match)) != "juanma-id" {
		t.Fatalf("partial Juanma match = %#v, error=%v", match, err)
	}
	match, err = resolveAssignableProjectMember(members, members, "jose")
	if err != nil || relationshipID(memberUser(match)) != "jose-id" {
		t.Fatalf("accent-folded José match = %#v, error=%v", match, err)
	}
	match, err = resolveAssignableProjectMember(members, members, "JUAN@EXAMPLE.COM")
	if err != nil || relationshipID(memberUser(match)) != "juan-perez-id" {
		t.Fatalf("email match = %#v, error=%v", match, err)
	}
	if _, err := resolveAssignableProjectMember(members, members, "juan"); err == nil || !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "Juanma Espinosa") || !strings.Contains(err.Error(), "Juan Pérez") {
		t.Fatalf("ambiguous partial match error = %v", err)
	}
}

func TestExactNormalizedMemberMatchTakesPrecedenceOverPartialMatches(t *testing.T) {
	members := []map[string]any{
		{"user": map[string]any{"id": "ann-id", "label": "Ann"}, "role": "developer"},
		{"user": map[string]any{"id": "anna-id", "label": "Anna Smith"}, "role": "qa"},
	}
	match, err := resolveAssignableProjectMember(members, members, "ANN")
	if err != nil || relationshipID(memberUser(match)) != "ann-id" {
		t.Fatalf("exact normalized match = %#v, error=%v", match, err)
	}
}

func TestAssignTaskResolvesPartialPersonAndRejectsAmbiguousPerson(t *testing.T) {
	ws, dir := makeSyncedTask(t, map[string]any{
		"id": "task-person-match", "number": float64(92), "title": "Assign by person",
		"type": "feature", "priority": "medium", "status": "toDo", "assignees": []any{}, "description": "",
	})
	members := []map[string]any{
		{"user": map[string]any{"id": "juanma-id", "label": "Juanma Espinosa", "email": "juanma@example.com"}, "role": "developer"},
		{"user": map[string]any{"id": "juan-perez-id", "label": "Juan Perez", "email": "juan@example.com"}, "role": "qa"},
	}
	if err := writeProjectMembers(ws, members[:1]); err != nil {
		t.Fatal(err)
	}
	if err := assignTask(ws, []string{"92", "juan"}, strings.NewReader(""), io.Discard); err != nil {
		t.Fatal(err)
	}
	metadata, err := workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := assigneesDisplay(metadata["assignees"]); got != "Juanma Espinosa" {
		t.Fatalf("assigned people = %q, want Juanma Espinosa", got)
	}

	if err := writeProjectMembers(ws, members); err != nil {
		t.Fatal(err)
	}
	if err := assignTask(ws, []string{"92", "juan"}, strings.NewReader(""), io.Discard); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous partial assignment error = %v", err)
	}
	metadata, err = workspace.ReadMap(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := assigneesDisplay(metadata["assignees"]); got != "Juanma Espinosa" {
		t.Fatalf("ambiguous assignment changed existing assignees: %q", got)
	}
}
