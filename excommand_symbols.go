package editorleaf

import (
	"context"
	"fmt"
	"strings"

	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/theme"
)

// Commands that list symbols or reformat, backed by the language service:
//
//	ESC x lsp-document-symbols  the outline of this buffer; choose one to jump to it
//	ESC x lsp-workspace-symbol Q  symbols of the workspace matching Q; choose one to jump
//	ESC x lsp-format            format the whole buffer with the language server
//
// Jumping leaves a mark first (see gotoLocation), so the usual mark commands
// go back.

func (e *Editorleaf) symbolsCommand() *Command {
	return &Command{
		CommandName: "lsp-document-symbols",
		// CommandAliases:     []string{"outline", "document-symbols", "lsp-symbols"},
		CommandDescription: "List the symbols of this buffer and jump to one",
		Run: func(ctx context.Context, args []string) (bool, error) {
			e.documentSymbols()
			return true, nil
		},
	}
}

func (e *Editorleaf) workspaceSymbolCommand() *Command {
	return &Command{
		CommandName: "lsp-workspace-symbol",
		// CommandAliases:     []string{"symbol", "lsp-workspace-symbol"},
		CommandDescription: "List the symbols of the workspace that match a query (lsp-workspace-symbol QUERY)",
		Run: func(ctx context.Context, args []string) (bool, error) {
			// ESC x passes the raw input, command name included.
			e.workspaceSymbols(commandText(args, "lsp-workspace-symbol"))
			return true, nil
		},
	}
}

func (e *Editorleaf) formatCommand() *Command {
	return &Command{
		CommandName: "lsp-format",
		// CommandAliases:     []string{"lsp-format"},
		CommandDescription: "Format the whole buffer with the language server",
		Run: func(ctx context.Context, args []string) (bool, error) {
			e.format()
			return true, nil
		},
	}
}

// ---- symbols ----

func (e *Editorleaf) documentSymbols() {
	if languageService == nil {
		gecore.Echo.AddText("No language service")
		return
	}
	if listPicker == nil {
		gecore.Echo.AddText("No list picker")
		return
	}

	eb := e.editBuffer
	version := eb.Version()
	languageService.DocumentSymbols(eb, func(symbols []Symbol, err error) {
		if err != nil {
			echoError(err)
			return
		}
		// The outline is of the text as it was: positions of a changed text
		// would send the cursor to the wrong place.
		if e.editBuffer != eb || eb.Version() != version {
			return
		}
		if len(symbols) == 0 {
			gecore.Echo.AddText("No symbols")
			return
		}

		lines := make([]string, len(symbols))
		for i, s := range symbols {
			lines[i] = outlineLine(s)
		}
		e.pickSymbol(fmt.Sprintf("Symbols (%d): ", len(symbols)), lines, symbols, eb, version)
	})
}

func (e *Editorleaf) workspaceSymbols(query string) {
	if languageService == nil {
		gecore.Echo.AddText("No language service")
		return
	}
	if listPicker == nil {
		gecore.Echo.AddText("No list picker")
		return
	}

	eb := e.editBuffer
	version := eb.Version()
	languageService.WorkspaceSymbols(eb, query, func(symbols []Symbol, err error) {
		if err != nil {
			echoError(err)
			return
		}
		// Symbols in this very buffer are positioned in the text as it was.
		if e.editBuffer != eb || eb.Version() != version {
			return
		}
		if len(symbols) == 0 {
			gecore.Echo.AddText("No symbols found")
			return
		}

		lines := make([]string, len(symbols))
		for i, s := range symbols {
			lines[i] = workspaceSymbolLine(s)
		}
		e.pickSymbol(fmt.Sprintf("Symbols (%d): ", len(symbols)), lines, symbols, eb, version)
	})
}

// pickSymbol lets the user choose a symbol and jumps to it, unless the buffer
// was edited while the list was open (for a symbol in the buffer itself that
// would land in the wrong place).
func (e *Editorleaf) pickSymbol(title string, lines []string, symbols []Symbol, eb *editbuffer.EditBuffer, version uint64) {
	listPicker(title, lines, func(i int) {
		if i < 0 || i >= len(symbols) {
			return
		}
		if eb.Version() != version {
			gecore.Echo.AddText("The buffer changed; jump cancelled").WithStyle(theme.ColorEchoRed)
			return
		}
		if err := e.gotoLocation(symbols[i].Location); err != nil {
			echoError(err)
		}
	})
}

// outlineLine is one line of an outline: indented by depth, with the kind and
// the line number.
func outlineLine(s Symbol) string {
	line := strings.Repeat("  ", s.Depth) + s.Name
	if s.Kind != "" {
		line += "  " + s.Kind
	}
	return fmt.Sprintf("%s  :%d", line, s.Location.Pos.RowIndex+1)
}

// workspaceSymbolLine is one line of a workspace symbol list: the name, its
// kind and container, and where it is.
func workspaceSymbolLine(s Symbol) string {
	line := s.Name
	if s.Kind != "" {
		line += "  " + s.Kind
	}
	if s.Container != "" {
		line += "  " + s.Container
	}
	return fmt.Sprintf("%s  %s:%d", line, shortPath(s.Location.Path), s.Location.Pos.RowIndex+1)
}

// ---- format ----

func (e *Editorleaf) format() {
	if languageService == nil {
		gecore.Echo.AddText("No language service")
		return
	}

	eb := e.editBuffer
	languageService.Format(eb, func(res FormatResult, err error) {
		if err != nil {
			echoError(err)
			return
		}
		if e.editBuffer != eb || eb.Version() != res.Version {
			gecore.Echo.AddText("Format cancelled: the buffer changed").WithStyle(theme.ColorEchoRed)
			return
		}
		if len(res.Edits) == 0 {
			gecore.Echo.AddText("Already formatted")
			return
		}

		// Everything is checked against the very rows the edits will be
		// applied to before the first one is made.
		edits, err := validateEdits(rowsOf(eb), res.Edits)
		if err != nil {
			echoError(fmt.Errorf("format: %w", err))
			return
		}

		pos := e.meta.RowsPos
		undo := eb.UndoAction
		first := undo.Index()
		undo.Seal() // do not merge into what the user typed before

		for j := len(edits) - 1; j >= 0; j-- { // bottom to top
			ed := edits[j]
			e.replaceRange(ed.Start, ed.End, ed.NewText)
		}
		undo.MergeSince(first) // one undo step for the whole formatting

		e.SetRowsPos(cursorAfterFormat(pos, edits))
		gecore.Echo.AddText(fmt.Sprintf("Formatted (%d edits)", len(edits)))
	})
}
