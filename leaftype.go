package editorleaf

import (
	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/screen"
	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/keychord"
)

type EditorKeymapBinder func(root *keychord.RootNode, editor *Editorleaf)

func NewLeafType(bind EditorKeymapBinder) *LeafType {
	return &LeafType{
		name:       "editorleaf",
		bindKeymap: bind,
	}
}

// Implements LeafType interface
type LeafType struct {
	name       string
	bindKeymap EditorKeymapBinder
	ctx        tree.LeafContext
}

// Return *editorleaf.Editor as tree.Leaf *interface
// Editor が生成されたタイミングで keymap を作成してバインドする
func (l *LeafType) NewLeaf() tree.Leaf {
	editor := newEditorLeaf()
	editor.parentLeafType = l
	editor.screen = screen.Get()

	// Editor が生成されたタイミングで keymap を作成してバインド
	km := keychord.NewRootNode()
	if l.bindKeymap != nil {
		l.bindKeymap(km, editor)
	}
	editor.SetKeyDispatcher(km)

	var leaf tree.Leaf = editor
	return leaf
}

// Create a new tree.Leaf (Editor) from leaf *tree.Leaf information
// direction: "right", "bottom"
func (l *LeafType) NewSiblingLeaf(direction string, leaf tree.Leaf) tree.Leaf {
	newEditor := newEditorLeaf()
	newEditor.parentLeafType = l
	newEditor.screen = screen.Get()

	// Editor が生成されたタイミングで keymap を作成してバインド
	km := keychord.NewRootNode()
	if l.bindKeymap != nil {
		l.bindKeymap(km, newEditor)
	}
	newEditor.SetKeyDispatcher(km)

	// Set the value of newEditor from leafEditor
	leafEditor := leaf.(*Editorleaf)
	// Same buffer pointer, copied meta. setBuffer also attaches the syntax
	// Worker for that buffer (newEditorLeaf attached one for BufferSets[0]).
	newEditor.setBuffer(leafEditor.editBuffer, leafEditor.meta.DeepCopy())

	// Cast to tree.Leaf interface and return
	var tl tree.Leaf = newEditor
	return tl
}

func (l *LeafType) RealName() string {
	return "github.com/ge-editor/editorleaf"
}

func (l *LeafType) Name() string {
	return l.name
}

func (l *LeafType) SetRegisteredName(name string) {
	l.name = name
}

func (l *LeafType) SetCtx(ctx *tree.LeafContext) {
	l.ctx = *ctx

	ctx.CancelManager.Rotate("draw").Done()
}

func (l *LeafType) CancelManager() *gecore.EventCancelManager {
	return l.ctx.CancelManager // .Get("draw")
}

// DispatchLeafRequest handles requests routed to the editor leaf type. The
// request is delivered to the Editorleaf that got keys last, when it is shown
// in the displayed RootTree, and otherwise to the first Editorleaf found in
// the tree; destination, behavior, and flag do not currently affect selection.
// After an OpenRequest that editor becomes the active leaf.
func (l *LeafType) DispatchLeafRequest(destination, behavior string, flag tree.OpenerFlag, data any, f func(bool, error)) {

	var e *Editorleaf
	if le := lastEditedEditor; le != nil && tree.TreeOfLeaf(le) != nil {
		e = le
	}
	if e == nil {
		tree.ForEachLeafAll(func(leaf tree.Leaf) bool {
			editor, ok := leaf.(*Editorleaf)
			if ok {
				e = editor
				return false // 打ちきり
			}
			return true
		})
	}

	if e == nil {
		f(false, nil)
		return
	}

	switch req := data.(type) {
	case tree.OpenRequest:
		handled, err := e.openTarget(req)
		// The editor that shows the file gets the keys from now on.
		if tr := tree.TreeOfLeaf(e); tr != nil {
			tree.SetActiveTree(tr)
		}
		f(handled, err)
	case tree.ReplaceRequest:
		// The dispatcher does not hand errors back to the sender, so they
		// are said here.
		if err := e.replaceRequest(req); err != nil {
			echoError(err)
		}
		f(true, nil)
	}
}

// openTarget opens req.Path in this editor and, when req.Line is given, moves
// the cursor there. The cursor is clamped to the buffer.
func (e *Editorleaf) openTarget(req tree.OpenRequest) (bool, error) {
	if _, err := e.OpenFile(req.Path); err != nil {
		return true, err
	}
	if req.Line > 0 {
		pos := editbuffer.RowsPos{RowIndex: req.Line - 1}
		if req.Col > 0 {
			pos.ColIndex = req.Col - 1
		}
		e.SetRowsPos(pos)
	}
	return true, nil
}
