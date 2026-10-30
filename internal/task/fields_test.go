package task

import "testing"

func TestComplexityLabelsAndEffortMappings(t *testing.T) {
	for _, complexity := range ComplexityOrder {
		effort, ok := ComplexityEffort[complexity]
		if !ok || effort.StoryPoints <= 0 || effort.Hours <= 0 {
			t.Errorf("complexity %q has no positive effort mapping", complexity)
		}
		if got := ComplexityLabel(complexity); got == complexity {
			t.Errorf("ComplexityLabel(%q) did not include effort", complexity)
		}
	}
	if got := ComplexityLabel("custom"); got != "custom" {
		t.Fatalf("unknown complexity label = %q", got)
	}
}
