package editorleaf

import (
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
}

// Return *editorleaf.Editor as tree.Leaf *interface
// Editor が生成されたタイミングで keymap を作成してバインドする
func (v *LeafType) NewLeaf() tree.Leaf {
	editor := newEditorLeaf()
	editor.parentLeafType = v
	editor.screen = screen.Get()

	// Editor が生成されたタイミングで keymap を作成してバインド
	km := keychord.NewRootNode()
	if v.bindKeymap != nil {
		v.bindKeymap(km, editor)
	}
	editor.SetKeyDispatcher(km)

	var leaf tree.Leaf = editor
	return leaf
}

// Create a new tree.Leaf (Editor) from leaf *tree.Leaf information
// direction: "right", "bottom"
func (v *LeafType) NewSiblingLeaf(direction string, leaf tree.Leaf) tree.Leaf {
	newEditor := newEditorLeaf()
	newEditor.parentLeafType = v
	newEditor.screen = screen.Get()

	// Editor が生成されたタイミングで keymap を作成してバインド
	km := keychord.NewRootNode()
	if v.bindKeymap != nil {
		v.bindKeymap(km, newEditor)
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

func (v *LeafType) Name() string {
	return v.name
}

// MinibufferLeaf は tree に属さない
// Editor が生成されたタイミングで keymap をバインドしない
func NewMinibufferLeaf() *Editorleaf {
	editor := newEditorLeaf()
	// ed.parentView = nil // tree に属さない leaf
	editor.screen = screen.Get()

	// keymap をバインドしない
	/*
		km := keychord.NewRootNode()
		if v.bindKeymap != nil {
			v.bindKeymap(km, editor)
		}
		editor.SetKeyDispatcher(km)
	*/

	return editor
}
