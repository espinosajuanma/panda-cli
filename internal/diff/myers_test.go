package diff

import "testing"

func TestMyersProducesShortestEditScript(t *testing.T) {
	got := Myers([]string{"same", "old", "tail"}, []string{"same", "new", "tail"})
	want := []Line{
		{Marker: ' ', Text: "same"},
		{Marker: '-', Text: "old"},
		{Marker: '+', Text: "new"},
		{Marker: ' ', Text: "tail"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}
