package report

import "testing"

func TestDisplayStatusAndCells(t *testing.T) {
	if got := DisplayStatus("inProgress"); got != "IN PROGRESS" {
		t.Fatalf("DisplayStatus = %q", got)
	}
	if got := TableCell("  hello \n world "); got != "hello world" {
		t.Fatalf("TableCell = %q", got)
	}
	if !IsEmptyField(nil) || !IsEmptyField("  ") || IsEmptyField("x") {
		t.Fatal("IsEmptyField did not treat blank values as empty")
	}
	if got := TruncateCell("abcdef", 4); got != "abc…" {
		t.Fatalf("TruncateCell = %q", got)
	}
}

func TestRelationshipDisplaysPreferCatalogNames(t *testing.T) {
	value := map[string]any{"id": "label-1", "label": "Fallback"}
	if got := RelationshipDisplayNameWithCatalog(value, map[string]string{"label-1": "Catalog"}); got != "Catalog" {
		t.Fatalf("catalog display = %q", got)
	}
	labels := []any{map[string]any{"id": "b", "label": "Beta"}, map[string]any{"id": "a", "label": "Alpha"}}
	if got := LabelsDisplayWithCatalog(labels, nil); got != "Alpha, Beta" {
		t.Fatalf("labels display = %q", got)
	}
	if got := AssigneesDisplay(labels); got != "Alpha, Beta" {
		t.Fatalf("assignees display = %q", got)
	}
}

func TestWrapValueSplitsLongWordsAndLines(t *testing.T) {
	lines := WrapValue("alpha abcdefghij beta", 4)
	want := []string{"alph", "a", "abcd", "efgh", "ij", "beta"}
	if len(lines) != len(want) {
		t.Fatalf("lines = %#v, want %#v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("lines = %#v, want %#v", lines, want)
		}
	}
}
