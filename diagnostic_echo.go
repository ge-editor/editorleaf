package editorleaf

import (
	"github.com/ge-editor/editorleaf/diagnostic"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/theme"
)

// echoDiagnosticAtCursor shows, in the echo area, the diagnostic under the
// cursor (the most severe one, with a count of the others). The echo line is
// cleared after every draw, so the message appears when the cursor arrives
// at the diagnostic, like other transient messages, and is appended to
// whatever a command has just echoed.
func (e *Editorleaf) echoDiagnosticAtCursor() {
	text, severity, ok := diagnostic.Summary(diagnostic.At(e.editBuffer, e.meta.RowsPos))
	if !ok {
		return
	}

	style := theme.ColorEchoLine
	switch severity {
	case diagnostic.Error:
		style = theme.ColorEchoRed
	case diagnostic.Warning:
		style = theme.ColorEchoYellow
	case diagnostic.Information:
		style = theme.ColorEchoBlue
	}
	gecore.Echo.AddText(text).WithStyle(style)
}
