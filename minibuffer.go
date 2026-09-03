// editorleaf/minibuffer.go

package editorleaf

import (
	"github.com/ge-editor/editorleaf/buffer"
	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore/screen"
	"github.com/ge-editor/gecore/styleresolver"
	"github.com/ge-editor/gelog"
	"github.com/ge-editor/locale"
)

func newMinibuffer() *Editorleaf {
	gelog.Debug("newMinibuffer")
	eb := editbuffer.NewFile("*minibuffer*")
	eb.New()
	e := &Editorleaf{
		screen:     screen.Get(),
		editBuffer: eb,
		meta:       buffer.NewMinibufferMeta(),
		mode:       ModeEditor,
		locale:     locale.New(),

		styleResolver: styleresolver.New(),
		// searchResolver:      &search.SearchResolver{},
		specialCharResolver: &styleresolver.SpecialCharResolver{},
	}
	e.bsArray = NewBoundariesArray(e)

	// e.styleResolver.Add(&search.SearchResolver{})
	e.styleResolver.Add(&styleresolver.SpecialCharResolver{})

	return e
}
