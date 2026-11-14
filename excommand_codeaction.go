package editorleaf

import (
	"context"
	"fmt"

	"github.com/ge-editor/gecore"
)

// ESC x lsp-code-action-quickfix
//
// Lists what the language server offers at the cursor (or at the region, if
// the mark is set: refactorings such as "extract function" work on a
// selection) and carries out the one chosen.
//
// Only actions that carry their edit are offered; the others the server may
// have offered are counted in the message. The edit is applied like a rename's
// (offerWorkspaceEdit): validated first, in the buffers' own undo history,
// unsaved, taken back together by workspace-edit-undo. An edit that only touches the
// current buffer is applied at once; one that reaches other files is shown for
// confirmation first.

func (e *Editorleaf) codeActionCommand() *Command {
	return &Command{
		CommandName: "lsp-code-action-quickfix",
		// CommandAliases:     []string{"actions", "quickfix", "lsp-code-action"},
		CommandDescription: "List the code actions at the cursor or region and apply one",
		Run: func(ctx context.Context, args []string) (bool, error) {
			e.codeActions()
			return true, nil
		},
	}
}

// codeActionLine is one line of the list: "Title [kind]", with * for the
// server's preferred action.
func codeActionLine(a CodeAction) string {
	line := a.Title
	if a.Kind != "" {
		line += "  [" + a.Kind + "]"
	}
	if a.Preferred {
		line = "* " + line
	}
	return line
}

func (e *Editorleaf) codeActions() {
	switch {
	case languageService == nil:
		gecore.Echo.AddText("No language service")
		return
	case listPicker == nil:
		gecore.Echo.AddText("No list picker")
		return
	}

	eb, pos := e.editBuffer, e.meta.RowsPos
	version := eb.Version()

	// The region, if there is one; otherwise the cursor.
	start, end := pos, pos
	if s, t, err := e.regionBounds(); err == nil {
		start, end = s, t
	}

	languageService.CodeActions(eb, start, end, func(actions []CodeAction, skipped int, err error) {
		if err != nil {
			echoError(err)
			return
		}
		// The actions are for the text and cursor as they were.
		if e.editBuffer != eb || eb.Version() != version || e.meta.RowsPos != pos {
			return
		}

		if len(actions) == 0 {
			msg := "No code actions"
			if skipped > 0 {
				msg += fmt.Sprintf(" (%d more not supported)", skipped)
			}
			gecore.Echo.AddText(msg)
			return
		}

		lines := make([]string, len(actions))
		for i, a := range actions {
			lines[i] = codeActionLine(a)
		}

		title := fmt.Sprintf("Code actions (%d): ", len(actions))
		listPicker(title, lines, func(i int) {
			if i < 0 || i >= len(actions) {
				return
			}
			a := actions[i]
			e.offerWorkspaceEdit(a.Title+": ", "Applied", a.Edit, false)
		})
	})
}
