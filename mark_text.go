package editorleaf

import (
	"bytes"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ge-editor/editorleaf/mark"
)

// The text of a mark.
//
// A mark stores its label (the text at the mark when it was set), and the
// label is not changed when the text is edited. Whether the Mark list shows
// the text that is at the mark now is chosen when ge is built:
//
//	go build            the text that is there now: from the buffer when the
//	                    file is open (the position of the mark follows the
//	                    edits), otherwise from the file. The stored label is
//	                    the fallback, for example for a file that is gone.
//	go build -tags marklabel
//	                    always the stored label (no buffer or file is read).
//
// The choice is the constant markTextLive (mark_text_live.go,
// mark_text_label.go).

const (
	markTextMaxFileSize = 4 << 20 // a larger file is not read: its label is used
	markTextMaxFiles    = 64
)

// MarkTextCache reads the files of closed buffers for MarkTextCache.Text. One
// cache serves one Mark list: call Begin before each rebuild of the list, so
// that each file is looked at (stat) once per rebuild and read only when it
// changed.
type MarkTextCache struct {
	gen   int
	files map[string]*markFile
}

type markFile struct {
	gen     int
	modTime time.Time
	size    int64
	lines   [][]byte
	ok      bool // false: the file cannot be used
}

func NewMarkTextCache() *MarkTextCache {
	return &MarkTextCache{files: map[string]*markFile{}}
}

// Begin starts a rebuild of the list.
func (c *MarkTextCache) Begin() { c.gen++ }

// Text returns the text at mk (the stored label when the build has the tag
// marklabel): from the position of the mark to the end of its
// row, or the whole row when nothing follows, with control characters and
// runs of blanks turned into one space. It must be called on the UI
// goroutine.
func (c *MarkTextCache) Text(mk *mark.Mark) string {
	if mk == nil {
		return ""
	}
	if !markTextLive || mk.ExtraData == nil {
		return mk.Label
	}

	line, found := openBufferLine(mk)
	if !found {
		line, found = c.diskLine(mk)
	}
	if found {
		if s := markLineText(line, mk.ColIndex); s != "" {
			return s
		}
	}
	return mk.Label
}

// openBufferLine returns the row of the mark in the open buffer of its file.
// found is false when the file is not open or the row is not there any more.
func openBufferLine(mk *mark.Mark) (line []byte, found bool) {
	if BufferSets == nil {
		return nil, false
	}
	for _, bs := range *BufferSets {
		if !sameMarkFile(mk.FilePath, bs.GetPath()) {
			continue
		}
		rows := bs.EditBuffer.Rows()
		if mk.RowIndex < 0 || mk.RowIndex >= rows.Length() {
			return nil, false
		}
		return rows.Row(mk.RowIndex).Bytes(), true
	}
	return nil, false
}

func (c *MarkTextCache) diskLine(mk *mark.Mark) (line []byte, found bool) {
	f := c.files[mk.FilePath]
	if f == nil || f.gen != c.gen {
		info, err := os.Stat(mk.FilePath)
		if err != nil || info.IsDir() || info.Size() > markTextMaxFileSize {
			c.files[mk.FilePath] = &markFile{gen: c.gen}
			return nil, false
		}
		if f != nil && f.ok && f.size == info.Size() && f.modTime.Equal(info.ModTime()) {
			f.gen = c.gen // unchanged since it was read
		} else {
			if len(c.files) >= markTextMaxFiles {
				clear(c.files)
			}
			f = &markFile{gen: c.gen, modTime: info.ModTime(), size: info.Size()}
			if b, err := os.ReadFile(mk.FilePath); err == nil {
				f.lines = bytes.Split(b, []byte{'\n'})
				f.ok = true
			}
			c.files[mk.FilePath] = f
		}
	}
	if !f.ok || mk.RowIndex < 0 || mk.RowIndex >= len(f.lines) {
		return nil, false
	}
	return bytes.TrimSuffix(f.lines[mk.RowIndex], []byte{'\r'}), true
}

// markLineText is the text of a row for the Mark list: from col to the end of
// the row, or the whole row when that is blank.
func markLineText(line []byte, col int) string {
	col = min(max(col, 0), len(line))
	for col > 0 && col < len(line) && !utf8.RuneStart(line[col]) {
		col-- // not in the middle of a character
	}
	if s := tidyMarkText(string(line[col:])); s != "" {
		return s
	}
	return tidyMarkText(string(line))
}

// tidyMarkText turns control characters and runs of blanks into one space and
// trims the ends.
func tidyMarkText(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if r < 0x20 || r == 0x7f || unicode.IsSpace(r) {
			if !space {
				b.WriteByte(' ')
				space = true
			}
			continue
		}
		b.WriteRune(r)
		space = false
	}
	return strings.TrimSpace(b.String())
}
