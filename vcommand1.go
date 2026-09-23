package editorleaf

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ge-editor/gelog"
)

func (e *Editorleaf) CommandPalette(s string) {
	input := strings.TrimSpace(s)
	if input == "" {
		return
	}

	switch {
	case strings.HasPrefix(input, "!"):
		cmd := strings.TrimSpace(input[1:])
		// 実行（例として標準出力）
		fmt.Printf("Run command: %s\n", cmd)
	case strings.HasPrefix(input, "|"):
		cmd := strings.TrimSpace(input[1:])
		// カーソル位置に挿入
		fmt.Printf("Insert at cursor: %s\n", cmd)
	default:
		i, err := strconv.Atoi(input)
		if err != nil {
			gelog.Error(err.Error())
			return
		}
		e.MoveCursorGoToLine(i)
	}
}
