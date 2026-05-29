// editorleaf/minibuffer.go

package editorleaf

import (
	"github.com/ge-editor/editorleaf/buffer"
	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore/screen"
	"github.com/ge-editor/locale"
)

func newMinibuffer() *Editorleaf {
	eb := editbuffer.NewFile("*minibuffer*")
	eb.New()
	e := &Editorleaf{
		screen:     screen.Get(),
		editBuffer: eb,
		meta:       buffer.NewMinibufferMeta(),
		mode:       ModeEditor,
		locale:     locale.New(),
	}
	e.bsArray = NewBoundariesArray(e)
	return e
}
