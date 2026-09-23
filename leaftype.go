package editorleaf

import (
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
	newEditor.editBuffer = leafEditor.editBuffer  // same pointer
	*newEditor.meta = *leafEditor.meta.DeepCopy() // copy value

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

// MinibufferLeaf は tree に属さない
// Editor が生成されたタイミングで keymap をバインドしない
/*
func NewMinibufferLeaf() *Editorleaf {
	editor := newEditorLeaf()
	// ed.parentView = nil // tree に属さない leaf
	editor.screen = screen.Get()

	// keymap をバインドしない

	// km := keychord.NewRootNode()
	// if v.bindKeymap != nil {
	// 	v.bindKeymap(km, editor)
	// }
	// editor.SetKeyDispatcher(km)

	return editor
}
*/
