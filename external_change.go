package editorleaf

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/keychord"
	"github.com/ge-editor/theme"
)

// ---------------------------------------------------------------------------
// A file that was changed by another program
// ---------------------------------------------------------------------------

// activeEditor returns the editor of the active leaf that edits a file, or
// nil (a minibuffer, an explorer, ...).
func activeEditor() *Editorleaf {
	t := tree.GetActiveTree()
	if t == nil {
		return nil
	}
	e, ok := t.GetLeaf().(*Editorleaf)
	if !ok || e == nil || e.mode != ModeEditor || e.editBuffer == nil {
		return nil
	}
	return e
}

// ActiveFileChangedOnDisk reports whether CheckExternalChange has something
// to ask: the file of the active editor was changed by another program and
// nothing else is being asked right now. It only looks at the file, so it
// can be called often. Call it with the UI locked.
func ActiveFileChangedOnDisk() bool {
	if MinibufferManager().IsActive() {
		return false
	}
	e := activeEditor()
	return e != nil && e.editBuffer.ChangedOnDisk()
}

// CheckExternalChange asks whether to reload the file of the active editor
// when another program changed it. Call it on the UI goroutine, and only when
// nothing else owns the keys (no mode is active).
//
// The question is asked once for a change: whatever the answer is (also C-g),
// the file is not asked about again until it changes once more. ESC x
// reload-file reloads it later.
func CheckExternalChange() {
	if !ActiveFileChangedOnDisk() {
		return
	}
	e := activeEditor()
	eb := e.editBuffer
	eb.IgnoreDiskChange()

	base := filepath.Base(eb.GetPath())
	prompt := fmt.Sprintf("%s was changed on disk. Reload it? [y/n]: ", base)
	if eb.IsDirtyFlag() {
		prompt = fmt.Sprintf("%s was changed on disk and has unsaved changes. Discard them and reload? [y/n]: ", base)
	}

	// A key sequence that was started (C-x ...) must not go on after the
	// question.
	keychord.ResetAllRootNodes()

	mb := MinibufferManager()
	session := NewSession(prompt, func(km *keychord.RootNode, miniEditor *Editorleaf) {
		km.BindKeyEvent(func(ev tcell.EventKey) (string, keychord.KeyDispatchTransition) {
			switch ev.Str() {
			case "y", "Y":
				mb.Close()
				e.ReloadFile()
			case "n", "N":
				mb.Close()
				gecore.Echo.AddText("Not reloaded: " + base + " differs from the file on disk").
					WithStyle(theme.ColorEchoYellow)
			}
			return "", keychord.DispatchExecuted
		})
	})
	mb.Start(session, func(result keychord.KeyDispatchTransition) {})
}

// ReloadFile replaces the content of the buffer with the file on disk. The
// unsaved changes and the undo history of the buffer are discarded. Every
// leaf that shows the buffer keeps its cursor as near as it can.
func (e *Editorleaf) ReloadFile() {
	eb := e.editBuffer
	path := eb.GetPath()
	if err := eb.Reload(); err != nil {
		gecore.Echo.AddText("Cannot reload " + path + ": " + err.Error()).WithStyle(theme.ColorEchoRed)
		return
	}

	refreshLeaves(eb)

	gecore.Echo.AddText("Reloaded " + path).WithStyle(theme.ColorEchoGreen)
	echoLoadWarnings(eb)
}

// reloadFileCommand is the ESC x "reload-file" command: reload the file of
// the buffer from the disk, also when ge was not told that it changed.
func (e *Editorleaf) reloadFileCommand() *Command {
	cmd := &Command{
		CommandName:        "reload-file",
		CommandDescription: "Reload the file from the disk (discards unsaved changes)",
	}
	cmd.Run = func(ctx context.Context, args []string) (bool, error) {
		if e.editBuffer.GetPath() == "" {
			gecore.Echo.AddText("The buffer has no file").WithStyle(theme.ColorEchoYellow)
			return true, nil
		}
		e.ReloadFile()
		return true, nil
	}
	return cmd
}
