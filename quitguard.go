// editorleaf/quitguard.go
//
// ge/key.go
//   ge/mode/mode.go
//     ge/modes/quitting_mode.go
//       gecore/quitguard_manager.go (manage quitguard list)
//         editorleaf/editorleaf.go  (register to quitguard manager)
//           editorleaf/quitguard.go

package editorleaf

import (
	"fmt"

	"github.com/gdamore/tcell/v3"

	"github.com/ge-editor/editorleaf/buffer"
	"github.com/ge-editor/editorleaf/editbuffer"
	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/keychord"
	"github.com/ge-editor/theme"
)

// implements QuitGuardResolver interface
// in gecore/quitguard_manager.go
type quitGuard struct {
	bufferSets   *buffer.BufferSets
	currentIndex int

	MinibufferManager *MinibufferManagerStruct
	RootNode          *keychord.RootNode
}

func NewQuitGuard(bufferSets *buffer.BufferSets) gecore.QuitGuardResolver {
	return &quitGuard{
		bufferSets:        bufferSets,
		MinibufferManager: MinibufferManager(),
		RootNode:          keychord.NewRootNode(),
	}
}

func (qg *quitGuard) Priority() int {
	return 50
}

func (qg *quitGuard) Name() string {
	return "quitGuard"
}

func (qg *quitGuard) Keys() *keychord.RootNode {
	return qg.RootNode
}

func (qg *quitGuard) WillEnter() {
	qg.ConfirmFunc()
}

func (qg *quitGuard) WillExit() {
}

func (qg *quitGuard) Draw() bool {
	// これは不要、呼び出すと echo line が消える
	// qg.MinibufferManager.Draw(screen.Get().Screen)
	return false
}

func (qg *quitGuard) ConfirmFunc() {
	qg.currentIndex = 0
	qg.askNext()
}

func findMetaInTree(killTargetBuf *editbuffer.EditBuffer) *buffer.Meta {
	var meta *buffer.Meta // nil

	tree.ForEachLeafAll(func(l tree.Leaf) {
		e, ok := l.(*Editorleaf)
		if ok {
			// Use the meta the leaf is actually using. BufferSets.GetMeta
			// pops the buffer's stack, which is empty while a leaf shows
			// the buffer, and would give a new meta (position 0:0).
			if meta == nil && e.editBuffer == killTargetBuf {
				meta = e.meta
			}
		}
	})

	return meta
}

func (qg *quitGuard) askNext() {
	if qg.currentIndex >= len(*qg.bufferSets) {
		qg.MinibufferManager.Close()
		gecore.QuitGuardManager.HandleNextResolver()
		return
	}

	buffSet := (*qg.bufferSets)[qg.currentIndex]

	// Saves the state of the first editorleaf found in the tree.
	meta := findMetaInTree(buffSet.EditBuffer)
	if meta == nil {
		// Not shown in the tree: use the last stacked meta without
		// popping it. With no meta there is no position to save; do not
		// overwrite the saved one with 0:0.
		if ms := buffSet.GetMetas(); len(ms) > 0 {
			meta = ms[len(ms)-1]
		}
	}
	if meta != nil {
		saveStateIfFile(buffSet.GetPath(), meta.RowsPos)
	}

	if !buffSet.IsDirtyFlag() {
		qg.currentIndex++
		qg.askNext()
		return // Not modified
	}

	base := buffSet.GetBase()
	prompt := fmt.Sprintf("Save modified buffer %s? [y/n]: ", base)

	mbSession := NewSession(prompt, func(km *keychord.RootNode, miniEditor *Editorleaf) {
		km.BindKeyEvent(func(ev tcell.EventKey) (string, keychord.KeyDispatchTransition) {
			// gelog.Info("bindkey")
			switch ev.Str() {
			case "y", "Y":
				// save
				_, err := buffSet.Save()
				if err != nil {
					gecore.Echo.AddText("⚠" + err.Error() + " " + base).WithStyle(theme.ColorEchoRed)
				} else {
					qg.currentIndex++
					qg.askNext()
				}
			case "n", "N":
				qg.currentIndex++
				qg.askNext()
			default:
			}
			return "", keychord.DispatchExecuted
		})
	})

	qg.MinibufferManager.Start(mbSession, nil)
}
