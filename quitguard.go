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
	"github.com/ge-editor/gecore/screen"
	"github.com/ge-editor/gecore/tree"
	"github.com/ge-editor/keychord"
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

func (qg *quitGuard) Name() string             { return "quitGuard" }
func (qg *quitGuard) Keys() *keychord.RootNode { return qg.RootNode }
func (qg *quitGuard) WillEnter() {
	qg.ConfirmFunc()
}
func (qg *quitGuard) WillExit() {}
func (qg *quitGuard) Draw() {
	qg.MinibufferManager.Draw(screen.Get().Screen)
}

func (qg *quitGuard) ConfirmFunc() {
	qg.currentIndex = 0
	qg.askNext()
}

func findMetaInTree(killTargetBuf *editbuffer.EditBuffer) *buffer.Meta {
	var meta *buffer.Meta // nil

	tree.GetRootTree().ForEachLeaf(func(l tree.Leaf) {
		e, ok := l.(*Editorleaf)
		if ok {
			if e.editBuffer == killTargetBuf {
				meta = BufferSets.GetMeta(e.editBuffer)
				return
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
		// Use the popped first meta if not found in the tree.
		meta = buffSet.PopMeta()
	}
	saveState(buffSet.GetPath(), meta.Cursor)

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
					gecore.Echo.AddText("⚠" + err.Error() + " " + base).JustNowActive(gecore.EchoRed)
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
