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

func (e *Editorleaf) SetMarkAtCursor() {
	label := e.getContentWidthoutSpecialCharactor(e.meta.RowsPos, 20)
	newMark := mark.NewMark(e.editBuffer.GetPath(), e.meta.RowsPos, label)

	// gelog.Debug("marks.Marks.Items()", marks.Marks.Items())

	for _, a := range marks.Marks.Items() {
		existing := toEditorMark(a)
		if existing == nil {
			continue
		}

		if !mark.Equals(existing, newMark) {
			continue
		}

		gecore.Echo.AddText("Unset mark").WithStyle(theme.ColorEchoRed)
		marks.Marks.Remove(a)
		return
	}

	gecore.Echo.AddText("Set mark").WithStyle(theme.ColorEchoGreen)
	marks.Marks.Add(newMark)
}

func (e *Editorleaf) findLastByEditBuffer() *mark.Mark {
	for _, a := range marks.Marks.Items() {
		lastMark, ok := a.(*mark.Mark)
		if ok && utils.IsSameFile(lastMark.FilePath, e.editBuffer.GetPath()) {
			return lastMark
		}
	}
	return nil
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
			e.meta.RowsPos = tmpMarkPos.RowsPos
			tmpMarkPos = nil
			return
		}
		gecore.Echo.AddText("Already at the last mark").WithStyle(theme.ColorEchoYellow)
		return
	} else {
		tmpMarkPos = mark.NewMark(e.editBuffer.GetPath(), e.meta.RowsPos, e.getContentWidthoutSpecialCharactor(e.meta.RowsPos, 20))
		e.meta.RowsPos = lastMark.RowsPos
	}
}

func (e *Editorleaf) FilterByCharacters(chars string) []*mark.Mark {
	items := []*mark.Mark{}

	for _, a := range marks.Marks.Items() {
		mk := toEditorMark(a)
		if mk == nil {
			continue
		}

		// text := fmt.Sprintf("%s %s", mk.EditBuffer.GetBase(), utils.RemoveSymbols(mk.Label))
		text := fmt.Sprintf("%s %s", filepath.Base(mk.FilePath), utils.RemoveSymbols(mk.Label))
		if chars != "" {
			if utils.ContainsAllCharacters(text, chars) {
				items = append(items, mk)
			}
		} else {
			items = append(items, mk)
		}
	}

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

// openMarkPosition prefers the workspace and RootTree recorded by the mark,
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
		root := canonicalPath(dir)
		rel, err := filepath.Rel(root, file)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

func canonicalPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	abs = filepath.Clean(abs)
	candidate := abs
	var suffix []string
	for {
		if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved)
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
		suffix = append(suffix, filepath.Base(candidate))
		candidate = parent
	}
	return abs
}
