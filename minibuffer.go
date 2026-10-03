// editorleaf/minibuffer.go

package editorleaf

import (
	"github.com/ge-editor/editorleaf/buffer"
	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/editorleaf/highlight"
	"github.com/ge-editor/gecore/screen"
	"github.com/ge-editor/locale"
)

func newMinibuffer() *Editorleaf {
	// gelog.Debug("newMinibuffer")

	eb := editbuffer.NewFile("*minibuffer*")
	eb.New()
	e := &Editorleaf{
		screen:     screen.Get(),
		editBuffer: eb,
		meta:       buffer.NewMinibufferMeta(),
		mode:       ModeEditor,
		locale:     locale.New(),

		highlightLayer: &highlight.HighlightsLayer{},
	}
	e.bsArray = NewBoundariesArray(e)
	e.InitTreesitter()

	return e
}
