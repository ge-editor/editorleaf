package spell

import "testing"

func withDefaults(t *testing.T, on, identifiers map[string]bool) {
	t.Helper()
	oldOn, oldIds := OnByDefault, IdentifiersByDefault
	OnByDefault, IdentifiersByDefault = on, identifiers
	t.Cleanup(func() { OnByDefault, IdentifiersByDefault = oldOn, oldIds })
}

func TestDefaultsAreOff(t *testing.T) {
	for _, mode := range []string{"Go", "Markdown", "Org", "Fundamental", ""} {
		if OnAtStart(mode) || IdentifiersAtStart(mode) {
			t.Errorf("%q starts with the spell check on", mode)
		}
	}
}

func TestDefaultsByMode(t *testing.T) {
	withDefaults(t, map[string]bool{"markdown": true, "go": true}, map[string]bool{"go": true})

	for mode, want := range map[string]bool{
		"Markdown": true, "markdown": true, "Go": true, // the case does not matter
		"Org": false, "Fundamental": false,
	} {
		if got := OnAtStart(mode); got != want {
			t.Errorf("OnAtStart(%q) = %v", mode, got)
		}
	}
	if !IdentifiersAtStart("Go") || IdentifiersAtStart("Markdown") {
		t.Error("IdentifiersAtStart")
	}
}

func TestDefaultsStarWithException(t *testing.T) {
	withDefaults(t, map[string]bool{"*": true, "go": false}, map[string]bool{"*": true})

	if !OnAtStart("Markdown") || !OnAtStart("Anything") {
		t.Error("* is every mode")
	}
	if OnAtStart("Go") {
		t.Error("a mode that is listed wins over *")
	}
	if !IdentifiersAtStart("Go") {
		t.Error("* for the identifiers")
	}
}
