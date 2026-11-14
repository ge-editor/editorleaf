package editorleaf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ge-editor/editorleaf/mark"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/marks"
	"github.com/ge-editor/gecore/pathscope"
	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/gecore/workspace"
	"github.com/ge-editor/gelog"
	"github.com/ge-editor/theme"
	"github.com/ge-editor/utils"
)

func (e *Editorleaf) MoveToMarkPosition(m *mark.Mark) {
	e.meta.RowsPos = m.RowsPos
	e.meta.ClampRowsPos(e.editBuffer)
}

// sameMarkFile reports whether a mark's file is the file of the buffer. It is
// called for every mark on every edit, so the paths are compared as strings
// first and the file system is asked only when the base names agree. The base
// names are compared ignoring case here, because on a file system that ignores
// case "Foo.go" and "foo.go" are one file; IsSameFile then decides, so on a
// case-sensitive file system they stay two files. It also works for a file
// that does not exist yet (IsSameFile compares the paths).
func sameMarkFile(markPath, bufferPath string) bool {
	if markPath == bufferPath {
		return true
	}
	if !strings.EqualFold(filepath.Base(markPath), filepath.Base(bufferPath)) {
		return false
	}
	return utils.IsSameFile(markPath, bufferPath)
}

func toEditorMark(a marks.Mark) *mark.Mark {

	switch v := a.(type) {
	case *mark.Mark:
		return v

	case *marks.StoredMark:
		var extra mark.ExtraData
		if err := json.Unmarshal(v.ExtraData, &extra); err != nil {
			gelog.Debug(err.Error())
			return nil
		}

		return &mark.Mark{
			ID:                v.ID,
			Label:             v.Label,
			WorkspaceID:       v.WorkspaceID,
			WorkspaceName:     v.WorkspaceName,
			RootTreeName:      v.RootTreeName,
			WorkspaceFilePath: v.WorkspaceFilePath,
			ExtraData:         &extra,
		}

	default:
		return nil
	}
}

// SetMarkAtCursor sets a mark at the cursor, or unsets the marks that are
// there.
//
// A mark is "there" when it is in the same file at the same position,
// whatever its scope (workspace, root tree) and its ID are. The marks of
// other scopes are in the same list, and marks that were saved or imported
// more than once are in it more than once. If only one of them were removed,
// the next call would find another and the position could never be cleared,
// so all of them are removed together.
func (e *Editorleaf) SetMarkAtCursor() {
	path := e.editBuffer.GetPath()
	pos := e.meta.RowsPos

	var ids []string
	for _, a := range marks.Marks.Items() {
		existing := toEditorMark(a)
		if existing == nil || existing.ExtraData == nil {
			continue
		}
		if existing.RowsPos != pos || !sameMarkFile(existing.FilePath, path) {
			continue
		}
		ids = append(ids, a.GetID())
	}

	if len(ids) > 0 {
		n := marks.Marks.RemoveByIDs(ids...)
		msg := "Unset mark"
		if n > 1 {
			msg = fmt.Sprintf("Unset mark (%d duplicates)", n)
		}
		gecore.Echo.AddText(msg).WithStyle(theme.ColorEchoRed)
		return
	}

	label := e.getContentWidthoutSpecialCharactor(pos, 20)
	gecore.Echo.AddText("Set mark").WithStyle(theme.ColorEchoGreen)
	marks.Marks.Add(mark.NewMark(path, pos, label))
}

// lastMarkOfFile returns the newest mark of the file (the marks are kept
// newest first), or nil. This is the one place that decides which mark is
// "the last mark": the region commands (M-w, C-w, ...) and the orange
// highlight (refreshMarkHighlights) must agree. Marks that were loaded from
// marks.json are StoredMarks, so they are converted with toEditorMark, and
// the file is compared with sameMarkFile as it is for the highlight.
func lastMarkOfFile(path string) *mark.Mark {
	return lastMarkIn(marks.Marks.Items(), path)
}

// lastMarkIn is lastMarkOfFile for a given list of marks (newest first).
func lastMarkIn(items []marks.Mark, path string) *mark.Mark {
	for _, a := range items {
		m := toEditorMark(a)
		if m == nil || m.ExtraData == nil || !sameMarkFile(m.FilePath, path) {
			continue
		}
		return m
	}
	return nil
}

func (e *Editorleaf) findLastByEditBuffer() *mark.Mark {
	return lastMarkOfFile(e.editBuffer.GetPath())
}

var tmpMarkPos *mark.Mark

func (e *Editorleaf) SwapCursorAndMark() {
	lastMark := e.findLastByEditBuffer()
	if lastMark == nil {
		gecore.Echo.AddText("No mark set").WithStyle(theme.ColorEchoYellow)
		return
	}

	if lastMark.RowsPos == e.meta.RowsPos {
		if tmpMarkPos != nil {
			e.SetRowsPos(tmpMarkPos.RowsPos)
			tmpMarkPos = nil
			return
		}
		gecore.Echo.AddText("Already at the last mark").WithStyle(theme.ColorEchoYellow)
		return
	} else {
		tmpMarkPos = mark.NewMark(e.editBuffer.GetPath(), e.meta.RowsPos, e.getContentWidthoutSpecialCharactor(e.meta.RowsPos, 20))
		e.SetRowsPos(lastMark.RowsPos)
	}
}

// FilterMarks lists the marks for the Mark mode. m restricts the marks to a
// PathScope (nil: no restriction): a mark is in the scope when its workspace
// ID or its file matches (see pathscope.Matcher). chars is the text filter and
// is applied to the marks in the scope.
//
// total is the number of all marks and inScope the number of marks in the
// scope, both before the text filter.
func (e *Editorleaf) FilterMarks(chars string, m *pathscope.Matcher) (items []*mark.Mark, inScope, total int) {
	return e.FilterMarksWith(chars, m, nil)
}

// FilterMarksWith is FilterMarks with the text that the text filter is
// applied to given by textOf. nil means the file name and the label of the
// mark as it was stored.
func (e *Editorleaf) FilterMarksWith(chars string, m *pathscope.Matcher, textOf func(*mark.Mark) string) (items []*mark.Mark, inScope, total int) {
	items = []*mark.Mark{}

	if textOf == nil {
		textOf = func(mk *mark.Mark) string {
			return fmt.Sprintf("%s %s", filepath.Base(mk.FilePath), utils.RemoveSymbols(mk.Label))
		}
	}

	for _, a := range marks.Marks.Items() {
		mk := toEditorMark(a)
		if mk == nil {
			continue
		}
		total++
		if m != nil && !m.Match(mk.WorkspaceID, mk.FilePath) {
			continue
		}
		inScope++

		// textOf is called for every mark in the scope (also without a text
		// filter): the caller may collect what it needs for the display.
		text := textOf(mk)
		if chars != "" {
			if utils.ContainsAllCharacters(text, chars) {
				items = append(items, mk)
			}
		} else {
			items = append(items, mk)
		}
	}

	return items, inScope, total
}

// FilterByCharacters lists the marks that match the text filter, in every
// scope.
func (e *Editorleaf) FilterByCharacters(chars string) []*mark.Mark {
	items, _, _ := e.FilterMarks(chars, nil)
	return items
}

////////////////////////////

// マーク位置が編集中のファイルの場合にはマーク位置に移動
// 現在編集中のウインドウ内にマークファイルを開きマーク位置に移動する。
func (e *Editorleaf) OpenMarkPositionInCurrentBuffer(mk *mark.Mark) error {
	if utils.IsSameFile(e.GetPath(), mk.FilePath) {
		// Set mark to Editor.mark if same the file
		e.MoveToMarkPosition(mk)
	} else {
		// Change the edit buffer
		file, _, result, err := BufferSets.GetFileAndMeta(mk.FilePath)
		if err != nil {
			return err
		}
		gecore.Echo.AddText(result.String())
		e.OpenFile(file.GetPath())
	}
	e.SetRowsPos(mk.RowsPos)
	return nil
}

// OpenMarkPosition prefers the workspace and RootTree recorded by the mark,
// then uses the current workspace when the marked file is under one of its
// directories, and finally falls back to the invoking editor.
func (e *Editorleaf) OpenMarkPosition(mk *mark.Mark) error {
	path := mk.FilePath
	e.OpenFile(path)

	originalRoot := tree.CurrentRootTree()
	originalWorkspace := workspace.Of(originalRoot)
	fallback := func() error {
		if current := tree.CurrentRootTree(); current != originalRoot && originalRoot != nil && originalWorkspace != nil {
			_ = workspace.Switch(workspace.Entry{Workspace: originalWorkspace, RootTree: originalRoot})
		}
		// Preserve the active workspace from before attempting to load the
		// recorded workspace file.
		if originalWorkspace != nil && pathIsInWorkspace(path, originalWorkspace) {
			return moveEditorToMark(e, path, mk)
		}
		return moveEditorToMark(e, path, mk)
	}

	if w := workspaceForMark(mk); w != nil {
		rt := rootTreeForMark(w, mk.RootTreeName)
		if rt != nil {
			if target, node := editorInRootTree(rt, path); target != nil {
				if !utils.IsSameFile(target.GetPath(), path) {
					if _, err := target.OpenFile(path); err != nil {
						return fallback()
					}
				}
				if err := workspace.Switch(workspace.Entry{Workspace: w, RootTree: rt}); err == nil {
					if rt.Active != node {
						tree.SetActiveTree(node)
					}
					target.SetRowsPos(mk.RowsPos)
					return nil
				}
			}
		}
	}
	return fallback()
}

func workspaceForMark(mk *mark.Mark) *workspace.Workspace {
	// The ID is the stable reference: it survives renames and moved files.
	if w := workspace.FindByID(mk.WorkspaceID); w != nil {
		return w
	}
	if mk.WorkspaceFilePath != "" {
		want := canonicalPath(mk.WorkspaceFilePath)
		for _, w := range workspace.All() {
			if w.FilePath != "" && canonicalPath(w.FilePath) == want {
				return w
			}
		}
		if _, err := os.Stat(mk.WorkspaceFilePath); err == nil {
			w, err := workspace.LoadFile(mk.WorkspaceFilePath)
			if err == nil {
				return w
			}
		}
		// A workspace with the same name but a different source file may be a
		// different project, so don't treat it as the recorded workspace.
		return nil
	}
	if mk.WorkspaceName != "" {
		return workspace.Find(mk.WorkspaceName)
	}
	return nil
}

func rootTreeForMark(w *workspace.Workspace, name string) *tree.RootTreeStruct {
	if w == nil || len(w.RootTrees) == 0 {
		return nil
	}
	for _, rt := range w.RootTrees {
		if rt.Name == name {
			return rt
		}
	}
	// The workspace is still a useful match if its saved RootTree was removed.
	return w.RootTrees[0]
}

func editorInRootTree(rt *tree.RootTreeStruct, path string) (*Editorleaf, *tree.Tree) {
	if rt == nil || rt.Root == nil {
		return nil, nil
	}
	var first *Editorleaf
	var firstNode *tree.Tree
	var matching *Editorleaf
	var matchingNode *tree.Tree
	rt.Root.Traverse(func(node *tree.Tree) {
		ed, ok := node.GetLeaf().(*Editorleaf)
		if !ok {
			return
		}
		if first == nil {
			first, firstNode = ed, node
		}
		if utils.IsSameFile(ed.GetPath(), path) {
			matching, matchingNode = ed, node
		}
	})
	if matching != nil {
		return matching, matchingNode
	}
	if rt.Active != nil {
		if ed, ok := rt.Active.GetLeaf().(*Editorleaf); ok {
			return ed, rt.Active
		}
	}
	return first, firstNode
}

func moveEditorToMark(target *Editorleaf, path string, mk *mark.Mark) error {
	if target == nil {
		return fmt.Errorf("no editor leaf is available for mark %q", mk.Label)
	}
	if !utils.IsSameFile(target.GetPath(), path) {
		if _, err := target.OpenFile(path); err != nil {
			return err
		}
	}
	target.SetRowsPos(mk.RowsPos)
	return nil
}

func pathIsInWorkspace(path string, w *workspace.Workspace) bool {
	if w == nil {
		return false
	}
	file := canonicalPath(path)
	for _, dir := range w.Directories {
		// IsWithinDir ignores case when the file system of the directory does.
		if utils.IsWithinDir(file, canonicalPath(dir)) {
			return true
		}
	}
	return false
}

func canonicalPath(path string) string {
	return pathscope.CanonicalPath(path)
}
