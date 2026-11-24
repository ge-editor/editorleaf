package editorleaf

import (
	"context"
	"strings"

	"github.com/ge-editor/editorleaf/charset"
	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/theme"
	"github.com/ge-editor/utils"
)

// The buffer holds UTF-8 with LF; the encoding and the newline that the file
// has are kept apart and written back by Save (see editbuffer/fileconv.go).
// These commands change them:
//
//	ESC x encoding NAME          the encoding that Save writes
//	ESC x reload-encoding NAME   read the file again as NAME
//	ESC x newline NAME           the newline that Save writes (lf, crlf, cr)

// encodingNames lists the encodings that contain all characters of input in
// order (case-insensitive). An empty input matches all.
func encodingNames(input string) []string {
	return filterNames(charset.SpecNames(), input)
}

var newlineNames = []string{"LF", "CRLF", "CR"}

func newlineCompletions(input string) []string {
	return filterNames(newlineNames, input)
}

func filterNames(all []string, input string) []string {
	input = strings.TrimSpace(input)
	names := []string{}
	for _, n := range all {
		if input == "" || utils.ContainsOrderedAllCharacters(n, input) {
			names = append(names, n)
		}
	}
	return names
}

// encodingCommand is the ESC x "encoding" command: it sets the encoding that
// the buffer is saved in. The text of the buffer does not change; the buffer
// is modified until it is saved. A character that the encoding cannot hold is
// reported now (with its line), not when saving.
func (e *Editorleaf) encodingCommand() *Command {
	cmd := &Command{
		CommandName:        "encoding",
		CommandDescription: "Set the encoding that the buffer is saved in",
		Complete:           encodingNames,
	}
	cmd.Run = func(ctx context.Context, args []string) (bool, error) {
		eb := e.editBuffer
		name := commandText(args, cmd.CommandName)
		if name == "" {
			gecore.Echo.AddText("Usage: encoding NAME (now " + eb.GetEncoding() + ")")
			return true, nil
		}
		spec, ok := charset.ParseSpec(name)
		if !ok {
			gecore.Echo.AddText("Unknown encoding: " + name).WithStyle(theme.ColorEchoRed)
			return true, nil
		}
		if eb.IsLossy() {
			gecore.Echo.AddText("The file was not read without loss and cannot be saved: " +
				"ESC x reload-encoding NAME reads it in another encoding").WithStyle(theme.ColorEchoYellow)
			return true, nil
		}
		if err := eb.CheckEncoding(spec); err != nil {
			gecore.Echo.AddText("Not changed: " + err.Error()).WithStyle(theme.ColorEchoRed)
			return true, nil
		}
		eb.SetEncoding(spec)
		gecore.Echo.AddText("Saved in " + eb.GetEncoding() + " from now on (not saved yet)")
		return true, nil
	}
	return cmd
}

// reloadEncodingCommand is the ESC x "reload-encoding" command: it reads the
// file again as the encoding says, for a file that was taken for another
// encoding. The buffer must not have unsaved changes (ESC x reload-file
// discards them).
func (e *Editorleaf) reloadEncodingCommand() *Command {
	cmd := &Command{
		CommandName:        "reload-encoding",
		CommandDescription: "Read the file again as the given encoding",
		Complete:           encodingNames,
	}
	cmd.Run = func(ctx context.Context, args []string) (bool, error) {
		eb := e.editBuffer
		name := commandText(args, cmd.CommandName)
		if name == "" {
			gecore.Echo.AddText("Usage: reload-encoding NAME (now " + eb.GetEncoding() + ")")
			return true, nil
		}
		spec, ok := charset.ParseSpec(name)
		if !ok {
			gecore.Echo.AddText("Unknown encoding: " + name).WithStyle(theme.ColorEchoRed)
			return true, nil
		}
		path := eb.GetPath()
		if path == "" {
			gecore.Echo.AddText("The buffer has no file").WithStyle(theme.ColorEchoYellow)
			return true, nil
		}
		if eb.IsDirtyFlag() {
			gecore.Echo.AddText("The buffer has unsaved changes: save it, or ESC x reload-file to discard them").
				WithStyle(theme.ColorEchoYellow)
			return true, nil
		}
		if err := eb.ReloadAs(spec); err != nil {
			gecore.Echo.AddText("Cannot reload " + path + ": " + err.Error()).WithStyle(theme.ColorEchoRed)
			return true, nil
		}
		refreshLeaves(eb)
		gecore.Echo.AddText("Reloaded " + path + " as " + eb.GetEncoding()).WithStyle(theme.ColorEchoGreen)
		echoLoadWarnings(eb)
		return true, nil
	}
	return cmd
}

// newlineCommand is the ESC x "newline" command: it sets the newline that the
// buffer is saved with. The buffer is modified until it is saved.
func (e *Editorleaf) newlineCommand() *Command {
	cmd := &Command{
		CommandName:        "newline",
		CommandDescription: "Set the newline that the buffer is saved with (lf, crlf, cr)",
		Complete:           newlineCompletions,
	}
	cmd.Run = func(ctx context.Context, args []string) (bool, error) {
		eb := e.editBuffer
		name := commandText(args, cmd.CommandName)
		if name == "" {
			gecore.Echo.AddText("Usage: newline lf|crlf|cr (now " + eb.GetNewLine().String() + ")")
			return true, nil
		}
		var nt editbuffer.NewlineType
		switch strings.ToLower(name) {
		case "lf", "unix":
			nt = editbuffer.NewlineTypeLF
		case "crlf", "dos", "windows":
			nt = editbuffer.NewlineTypeCRLF
		case "cr", "mac":
			nt = editbuffer.NewlineTypeCR
		default:
			gecore.Echo.AddText("Unknown newline: " + name + " (lf, crlf, cr)").WithStyle(theme.ColorEchoRed)
			return true, nil
		}
		eb.SetNewline(nt)
		gecore.Echo.AddText("Saved with " + eb.GetNewLine().String() + " from now on (not saved yet)")
		return true, nil
	}
	return cmd
}

// refreshLeaves makes every leaf that shows eb forget what it drew and keep
// its cursor as near as it can, after the rows were replaced.
func refreshLeaves(eb *editbuffer.EditBuffer) {
	tree.ForEachLeafAll(func(l tree.Leaf) bool {
		ed, ok := l.(*Editorleaf)
		if !ok || ed.editBuffer != eb || ed.meta == nil {
			return true
		}
		ed.bsArray.ClearAll()
		ed.meta.ClampRowsPos(eb)
		return true
	})
}

// echoLoadWarnings tells what the user should know about the file that was
// read: unified newlines, and a file that is read-only because it cannot be
// written back without loss.
func echoLoadWarnings(eb *editbuffer.EditBuffer) {
	const warn = editbuffer.ResultMixedNewline | editbuffer.ResultLossy | editbuffer.ResultBinary
	if s := (eb.LoadResult() & warn).String(); s != "" {
		gecore.Echo.AddText(s).WithStyle(theme.ColorEchoYellow)
	}
}
