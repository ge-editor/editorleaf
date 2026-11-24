package editorleaf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ge-editor/editorleaf/charset"
	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/theme"
	"github.com/ge-editor/utils"
)

// Rename across files.
//
//	ESC x lsp-rename-symbol NEWNAME   rename the symbol at the cursor
//	ESC x workspace-edit-undo         take the last rename (or code action) back, in all files
//
// How it is done, and why:
//
//   - Nothing is changed until everything has been checked. The edits come
//     from the language server for the text it saw; they are validated
//     against the text the editor has (positions inside the text, no
//     overlaps), and against the versions of the buffers they were computed
//     for. Any doubt is an error, and then nothing at all has been touched.
//   - The user sees a count and one line per changed row, with warnings, and
//     confirms. Only the "apply" line applies; anything else cancels.
//   - A file that is already a buffer is edited in that buffer. A file that is
//     not is loaded as a buffer first (never a second buffer for the same
//     file). All loading happens before the first edit, and if it fails the
//     buffers loaded so far are removed again.
//   - Every edit goes through the same code as typing (replaceRange), in the
//     buffer's own undo history, so every file is a normal modified buffer:
//     it is not saved, and a normal Undo works in it. The edits of one file
//     are merged into a single undo step. workspace-edit-undo takes all files
//     back in one command, provided none was edited since.

// renamedFile is a buffer touched by the last rename.
type renamedFile struct {
	eb         *editbuffer.EditBuffer
	path       string
	indexAfter int // the buffer's undo index right after the rename
}

type renameRecord struct {
	files []renamedFile
}

// lastRename is the rename that workspace-edit-undo would take back (UI goroutine only).
var lastRename *renameRecord

func (e *Editorleaf) renameCommand() *Command {
	return &Command{
		CommandName: "lsp-rename-symbol",
		// CommandAliases:     []string{"rename-symbol", "lsp-rename"},
		CommandDescription: "Rename the symbol at the cursor everywhere (lsp-rename-symbol NEWNAME)",
		Run: func(ctx context.Context, args []string) (bool, error) {
			e.rename(args)
			return true, nil
		},
	}
}

func (e *Editorleaf) renameUndoCommand() *Command {
	return &Command{
		CommandName: "workspace-edit-undo",
		// CommandAliases:     []string{"workspace-edit-undo"},
		CommandDescription: "Undo the last rename or code action in all the files it changed",
		Run: func(ctx context.Context, args []string) (bool, error) {
			if err := e.renameUndo(); err != nil {
				echoError(err)
			}
			return true, nil
		},
	}
}

// renameNewName returns the new name that was typed after the command name.
// ESC x passes the raw input ("lsp-rename-symbol NewName") as one element,
// command name included, like it does for every command; taking that whole
// string as the name sent "lsp-rename-symbol NewName" to the language server.
// Other callers may pass the name alone; both forms are accepted.
func renameNewName(args []string) string {
	return commandText(args, "lsp-rename-symbol")
}

// RenameUnavailableReason says why a rename cannot be done in this buffer, or
// returns "" if it can. It is for asking before the new name is asked for;
// rename checks it again.
func (e *Editorleaf) RenameUnavailableReason() string {
	switch {
	case languageService == nil:
		return "No language service"
	case previewPicker == nil:
		// The user must always see what a rename does before it is done.
		return "No preview picker: rename needs one to confirm"
	case e.GetPath() == "":
		return "Rename needs a buffer with a file"
	}

	if c, ok := languageService.(LanguageServerChecker); ok && !c.HasLanguageServer(e.editBuffer) {
		return "No language server for this buffer"
	}

	return ""
}

func (e *Editorleaf) rename(args []string) {
	newName := renameNewName(args)
	if newName == "" || strings.ContainsAny(newName, " \t") {
		gecore.Echo.AddText("Usage: lsp-rename-symbol NEWNAME (one word)")
		return
	}

	if reason := e.RenameUnavailableReason(); reason != "" {
		gecore.Echo.AddText(reason)
		return
	}

	eb, pos := e.editBuffer, e.meta.RowsPos
	version := eb.Version()
	languageService.Rename(eb, pos, newName, func(we *WorkspaceEdit, err error) {
		if err != nil {
			echoError(err)
			return
		}
		if e.editBuffer != eb || eb.Version() != version || e.meta.RowsPos != pos {
			gecore.Echo.AddText("Rename cancelled: the buffer or the cursor changed").WithStyle(theme.ColorEchoRed)
			return
		}

		e.offerWorkspaceEdit(fmt.Sprintf("Rename to %s: ", newName), "Renamed", we, true)
	})
}

// offerWorkspaceEdit applies we, after showing the user what it does and
// getting a confirmation (title is the prompt of that dialog, what the verb
// of the final message).
//
// The confirmation is skipped when the edit is confined to the current buffer
// and alwaysPreview is false: choosing a quick fix from a list is already the
// user's decision, and its effect is in view. Anything that reaches other
// files is always shown first.
func (e *Editorleaf) offerWorkspaceEdit(title, what string, we *WorkspaceEdit, alwaysPreview bool) {
	plan, err := e.planRename(we)
	if err != nil {
		echoError(err)
		return
	}
	if plan.editCount == 0 {
		gecore.Echo.AddText("Nothing to change")
		return
	}

	apply := func() {
		// The files are checked again: time has passed since the plan.
		if err := e.applyRename(we); err != nil {
			echoError(err)
			return
		}
		gecore.Echo.AddText(fmt.Sprintf(
			"%s: %d edits in %d files (not saved; workspace-edit-undo takes it back)",
			what, plan.editCount, len(plan.files)))
	}

	onlyHere := len(plan.files) == 1 && plan.files[0].eb == e.editBuffer
	if onlyHere && !alwaysPreview {
		apply()
		return
	}

	if previewPicker == nil {
		// The user must always see what a change to other files does first.
		gecore.Echo.AddText("No preview picker: it is needed to confirm")
		return
	}

	details := make([]string, 0, len(plan.warnings)+len(plan.preview))
	for _, w := range plan.warnings {
		details = append(details, "! "+w)
	}
	details = append(details, plan.preview...)

	label := fmt.Sprintf("Apply: %d edits in %d files", plan.editCount, len(plan.files))
	previewPicker(title, label, details, apply)
}

// ---- plan ----

type plannedFile struct {
	path  string
	edits []TextEdit             // validated, sorted
	eb    *editbuffer.EditBuffer // nil if the file is not open yet
	rows  [][]byte
}

type renamePlan struct {
	files     []plannedFile
	editCount int
	warnings  []string
	preview   []string
}

// findBuffer returns the open buffer of path, or nil.
func findBuffer(path string) *editbuffer.EditBuffer {
	for _, bs := range *BufferSets {
		if utils.IsSameFile(path, bs.GetPath()) {
			return bs.EditBuffer
		}
	}
	return nil
}

// planRename checks we against the present state of the buffers and files and
// describes what applying it would do. It changes nothing.
func (e *Editorleaf) planRename(we *WorkspaceEdit) (*renamePlan, error) {
	plan := &renamePlan{}
	seen := map[*editbuffer.EditBuffer]string{}
	var unopened []os.FileInfo

	for _, fe := range we.Files {
		if len(fe.Edits) == 0 {
			continue
		}
		name := shortPath(fe.Path)
		f := plannedFile{path: fe.Path}

		if eb := findBuffer(fe.Path); eb != nil {
			if other, dup := seen[eb]; dup {
				return nil, fmt.Errorf("%s and %s are the same file", other, name)
			}
			seen[eb] = name
			f.eb = eb
			f.rows = rowsOf(eb)

			// Edits computed for one text must not be applied to another.
			if fe.Open && eb.Version() != fe.Version {
				return nil, fmt.Errorf("%s changed since the rename was computed", name)
			}

			// Read like the buffer does (encoding, NFC), or the rows differ
			// from the buffer for any file that is not UTF-8.
			file, err := charset.ReadFile(fe.Path)
			switch {
			case err != nil:
				plan.warnings = append(plan.warnings, name+": the file is not on disk")
			case !sameRows(splitLines(file.Text), f.rows):
				if !fe.Open {
					// The server read the file from disk, but the buffer has
					// other content: its positions do not fit the buffer.
					return nil, fmt.Errorf("%s has unsaved changes and is not known to the language server: save or revert it first", name)
				}
				plan.warnings = append(plan.warnings, name+": the buffer differs from the file on disk (unsaved changes)")
			}
		} else {
			info, err := os.Stat(fe.Path)
			if err != nil || !info.Mode().IsRegular() {
				return nil, fmt.Errorf("cannot rename in %s: not a readable file", name)
			}
			for _, prev := range unopened {
				if os.SameFile(prev, info) {
					return nil, fmt.Errorf("%s is listed twice", name)
				}
			}
			unopened = append(unopened, info)

			file, err := charset.ReadFile(fe.Path)
			if err != nil {
				return nil, fmt.Errorf("cannot read %s: %w", name, err)
			}
			f.rows = splitLines(file.Text)
			plan.warnings = append(plan.warnings, name+": will be loaded as a buffer (not saved)")
		}

		sorted, err := validateEdits(f.rows, fe.Edits)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		f.edits = sorted

		plan.editCount += len(sorted)
		plan.preview = append(plan.preview, previewLines(name, f.rows, sorted)...)
		plan.files = append(plan.files, f)
	}
	return plan, nil
}

// ---- apply ----

// applyRename performs we. It plans again first, so that it works on what is
// true now, then makes the changes in two phases: loading everything that is
// needed (nothing is modified; on failure the loaded buffers are removed), and
// only then editing, which cannot fail any more because every edit has been
// validated against the very rows it will be applied to.
func (e *Editorleaf) applyRename(we *WorkspaceEdit) error {
	plan, err := e.planRename(we)
	if err != nil {
		return err
	}
	if plan.editCount == 0 {
		return errors.New("nothing to rename")
	}

	origPath := e.GetPath()
	origEB := e.editBuffer
	origPos := e.meta.RowsPos
	if origPath == "" {
		return errors.New("rename needs a buffer with a file")
	}

	// Phase 1: load the files that are not open. Nothing is edited yet.
	var loaded []*editbuffer.EditBuffer
	abort := func(err error) error {
		// Back to where we were, then drop what was loaded for this rename.
		_, _ = e.OpenFile(origPath)
		for _, eb := range loaded {
			BufferSets.RemoveByBufferFile(eb)
		}
		return err
	}
	for i := range plan.files {
		f := &plan.files[i]
		if f.eb != nil {
			continue
		}
		name := shortPath(f.path)

		result, err := e.OpenFile(f.path)
		// A file that cannot be loaded would come back as a new, empty
		// buffer: that must not stay around either.
		loaded = append(loaded, e.editBuffer)
		if err != nil || result != editbuffer.ResultLoadedFile {
			return abort(fmt.Errorf("cannot load %s", name))
		}

		f.eb = e.editBuffer
		f.rows = rowsOf(f.eb)
		if _, err := validateEdits(f.rows, f.edits); err != nil {
			return abort(fmt.Errorf("%s: %w", name, err))
		}
	}

	// Phase 2: edit. The pane visits each buffer, because that is where the
	// editing code works; it ends up back on the original file.
	rec := &renameRecord{}
	var originEdits []TextEdit

	for _, f := range plan.files {
		if _, err := e.OpenFile(f.path); err != nil || e.editBuffer != f.eb {
			e.rollbackRename(rec, origPath)
			return fmt.Errorf("cannot switch to %s; the rename was rolled back", shortPath(f.path))
		}

		undo := f.eb.UndoAction
		first := undo.Index()
		undo.Seal() // do not merge into what the user typed before

		for j := len(f.edits) - 1; j >= 0; j-- { // bottom to top
			ed := f.edits[j]
			e.replaceRange(ed.Start, ed.End, ed.NewText)
		}
		undo.MergeSince(first) // one undo step for the file

		rec.files = append(rec.files, renamedFile{eb: f.eb, path: f.path, indexAfter: undo.Index()})
		if f.eb == origEB {
			originEdits = f.edits
		}
	}

	if _, err := e.OpenFile(origPath); err == nil {
		e.SetRowsPos(cursorAfterEdits(origPos, originEdits))
	}
	lastRename = rec
	return nil
}

// rollbackRename undoes the files of rec (newest first) and returns to
// origPath. Used when a rename fails after it has started editing.
func (e *Editorleaf) rollbackRename(rec *renameRecord, origPath string) {
	for i := len(rec.files) - 1; i >= 0; i-- {
		if _, err := e.OpenFile(rec.files[i].path); err == nil {
			e.Undo()
		}
	}
	_, _ = e.OpenFile(origPath)
}

// ---- undo ----

func bufferIsOpen(eb *editbuffer.EditBuffer) bool {
	for _, bs := range *BufferSets {
		if bs.EditBuffer == eb {
			return true
		}
	}
	return false
}

// renameUndo takes the last rename back in every file it changed, as one
// operation: first all files are checked, and unless every one is exactly as
// the rename left it, nothing is undone.
func (e *Editorleaf) renameUndo() error {
	rec := lastRename
	if rec == nil {
		return errors.New("No rename to undo")
	}

	for _, f := range rec.files {
		name := shortPath(f.path)
		switch {
		case !bufferIsOpen(f.eb):
			return fmt.Errorf("Cannot undo the rename: %s was closed", name)
		case f.eb.UndoAction.Index() != f.indexAfter:
			return fmt.Errorf("Cannot undo the rename: %s was edited since", name)
		}
	}

	origPath := e.GetPath()
	for i := len(rec.files) - 1; i >= 0; i-- {
		if _, err := e.OpenFile(rec.files[i].path); err != nil {
			_, _ = e.OpenFile(origPath)
			return fmt.Errorf("cannot switch to %s", shortPath(rec.files[i].path))
		}
		e.Undo()
	}
	_, _ = e.OpenFile(origPath)

	lastRename = nil
	gecore.Echo.AddText(fmt.Sprintf("Rename undone in %d files (buffers loaded for it stay open)", len(rec.files)))
	return nil
}
