package spell

import "strings"

// What a buffer starts with. This is the place to change it.
//
// The keys are the lower case names of the language modes ("go", "markdown",
// "org", "fundamental"); "*" stands for every mode that is not listed. A mode
// that is listed wins over "*": {"*": true, "go": false} starts the check in
// every buffer but the Go ones.
//
// Both are off by default (the maps are empty). Nothing is checked until
// ESC x spell, and ESC x spell-identifiers turns the identifiers on. For
// example, to start with the spell check on for text and Markdown, and with
// the identifiers checked in Go files:
//
//	var OnByDefault = map[string]bool{
//		"markdown":    true,
//		"fundamental": true,
//		"org":         true,
//		"go":          true,
//	}
//	var IdentifiersByDefault = map[string]bool{"go": true}
//
// The spell check only starts if ge was built with the engine (-tags
// hunspell) and the dictionary is there; otherwise it stays off without a
// word.
var (
	// OnByDefault: the modes whose buffers start with the spell check on.
	OnByDefault = map[string]bool{
		// "markdown": true, "fundamental": true, "org": true,
		"*": true,
	}

	// IdentifiersByDefault: the modes in which the identifiers are checked
	// as soon as the spell check starts (by ESC x spell or by
	// OnByDefault). It has no effect in a mode that has no code, such as
	// Markdown: all of its text is checked.
	IdentifiersByDefault = map[string]bool{
		// "*": true, "go": false,
		"*": true,
	}
)

func startsOn(table map[string]bool, mode string) bool {
	if v, ok := table[strings.ToLower(strings.TrimSpace(mode))]; ok {
		return v
	}
	return table["*"]
}

// OnAtStart reports whether the buffers of a language mode (lang.Mode.Name())
// start with the spell check on.
func OnAtStart(mode string) bool { return startsOn(OnByDefault, mode) }

// IdentifiersAtStart reports whether the identifiers of a language mode are
// checked as soon as the spell check starts.
func IdentifiersAtStart(mode string) bool { return startsOn(IdentifiersByDefault, mode) }
