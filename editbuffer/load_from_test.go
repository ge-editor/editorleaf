package editbuffer

import (
	"strings"
	"testing"
)

func TestLoadFromDetectsNewlineAndKeepsFinalEmptyRow(t *testing.T) {
	eb := NewEditBuffer(nil)
	if err := eb.LoadFrom(strings.NewReader("a\r\nb\r\n")); err != nil {
		t.Fatal(err)
	}
	if got := eb.GetNewLine(); got != NewlineTypeCRLF {
		t.Fatalf("newline type = %v, want CRLF", got)
	}
	// "a", "b" and the empty row after the final newline, like Load.
	if got := eb.Rows().Length(); got != 3 {
		t.Fatalf("rows = %d, want 3", got)
	}
}

func TestLoadFromEmptyInputGivesOneEmptyRow(t *testing.T) {
	eb := NewEditBuffer(nil)
	if err := eb.LoadFrom(strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	if got := eb.Rows().Length(); got != 1 {
		t.Fatalf("rows = %d, want 1", got)
	}
}
