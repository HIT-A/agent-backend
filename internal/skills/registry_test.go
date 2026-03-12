package skills

import (
	"reflect"
	"testing"
)

func TestRegistry_BuiltInSkills_ContainsAtLeastOneSkill(t *testing.T) {
	r := NewRegistry()

	got := r.List()
	if len(got) < 1 {
		t.Fatalf("expected at least 1 built-in skill, got %d", len(got))
	}

	// Ensure the slice is stable/deterministic for callers.
	if !reflect.DeepEqual(got, r.List()) {
		t.Fatalf("expected List() to be deterministic")
	}
}
