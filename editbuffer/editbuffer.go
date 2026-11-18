package editbuffer

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ge-editor/editorleaf/charset"
	"github.com/ge-editor/gecore/lang"
	"github.com/ge-editor/utils"
)

type flags int8

const (
	READONLY flags = 1 << iota
)

type EditBuffer struct {
	rawPath  string
	path     string
	base     string
	ext      string
	dispPath string

	size    int64
	mode    os.FileMode
	modTime time.Time

	// disk is the state of the file when it was last read or written
	// (see diskstate.go).
	disk diskState

	langMode *lang.Mode

	rows *Rows_
	NewlineType

	// The file as it is on disk (see fileconv.go): the rows are UTF-8, the
	// codec says how to write them back.
	codec      fileCodec
	loadResult Result
	// metaDirty: the encoding or the newline was changed. That is not an
	// edit, so the undo stack does not know it; the buffer is modified
	// anyway until it is saved.
	metaDirty bool

	flags // readonly

	UndoAction *UndoStack

	// treesitter
	changeNotifier // buffer change notifications (see change.go)
}

// Call New() or Load() after invoking this function
func NewFile(rawPath string) *EditBuffer {
	langMode := lang.Modes.GetMode(rawPath)

	ff := &EditBuffer{
		rawPath:  rawPath,
		path:     "",
		base:     "",
		ext:      filepath.Ext(rawPath),
		dispPath: "",

		size:    0,
		mode:    fs.ModePerm,
		modTime: time.Now(),

		langMode: langMode,

		rows:        nil,
		NewlineType: NewlineTypeLF,

		flags: 0,

		UndoAction: NewUndoStack(),
	}
	ff.init()
	return ff
}

// Initialize File with File.rawPath
func (eb *EditBuffer) init() {
	if eb.rawPath == "" {
		eb.rawPath = "unnamed"
	}

	eb.size = 0
	eb.mode = fs.ModePerm
	eb.modTime = time.Now()
	info, err := os.Stat(eb.rawPath)
	if err == nil {
		if info.IsDir() {
			eb.rawPath = "unnamed"
		}
		eb.size = info.Size()
		eb.mode = info.Mode()
		eb.modTime = info.ModTime()
	}
	eb.path, err = filepath.Abs(eb.rawPath)
	if err != nil {
		eb.path = ""
	}

	dir := ""
	dir, eb.base = filepath.Split(eb.path)
	eb.dispPath = eb.base
	dir = utils.LastPartOfPath(dir)
	wd, err := os.Getwd()
	if err == nil {
		if utils.IsSameFile(wd, dir) {
			eb.dispPath = filepath.Join(dir, eb.dispPath)
		}
	}

	eb.ext = filepath.Ext(eb.path)
}

func NewEditBuffer(r *Rows_) *EditBuffer {
	return &EditBuffer{
		rows: r,
	}
}

func (eb *EditBuffer) Rows() *Rows_ {
	return eb.rows
}

/* func (eb *EditBuffer) ChangePath(path string) {
	eb.rawPath = path
	eb.init()
}
*/

// New file
func (eb *EditBuffer) New() error {
	eb.rows = NewRowsType()
	eb.rows.AddRow([]byte{})

	eb.NewlineType = NewlineTypeLF
	return nil
}

// Load file
func (eb *EditBuffer) Load() error {
	// Stat before reading, so a change during the read is not lost.
	mt, size, ok := statRegular(eb.path)

	raw, err := os.ReadFile(eb.path)
	if err != nil {
		return err
	}
	if err := eb.loadFile(charset.Open(raw)); err != nil {
		return err
	}
	eb.setDisk(mt, size, ok)
	return nil
}

// LoadFrom replaces the content with everything read from r, converted like
// Load does for the file: the encoding is found out and converted to UTF-8,
// and the newline type is detected.
func (eb *EditBuffer) LoadFrom(r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return eb.loadFile(charset.Open(raw))
}

// loadText replaces the content with the UTF-8 text read from r and detects
// the newline type.
func (eb *EditBuffer) loadText(r io.Reader) error {
	reader := bufio.NewReader(r)

	eb.loadResult = ResultNone
	eb.rows = NewRowsType()
	var countLF, countCRLF, countCR int
	var finalRowNewlineType NewlineType
	for {
		line, newlineType, err := ReadLine(reader)

		switch newlineType {
		case NewlineTypeLF:
			countLF++
		case NewlineTypeCRLF:
			countCRLF++
		case NewlineTypeCR:
			countCR++
		}

		// If only a newline, the size is zero.
		// if len(line) > 0 {
		b := make([]byte, len(line), len(line)+16)
		copy(b, line)
		eb.rows.AddRow(b)
		// }

		if err == io.EOF {
			finalRowNewlineType = newlineType
			break
		}
		if err != nil {
			return err
		}
	}

	if eb.rows.Length() == 0 || finalRowNewlineType != NewlineTypeNone {
		eb.rows.AddRow([]byte{})
	} /*  else {
		linesIndex := eb.RowsLength() - 1
		lineIndex := eb.Rows().Row(linesIndex).Length()
		if ch, _, _ := eb.Rows().Row(linesIndex).DecodeRune(lineIndex - 1); ch == '\n' {
			eb.Rows().Add([]byte{define.EOF})
		} else {
			eb.Rows().Row(linesIndex).Add([]byte{define.EOF})
		}
	} */

	kinds := 0
	for _, c := range []int{countLF, countCRLF, countCR} {
		if c > 0 {
			kinds++
		}
	}
	if kinds > 1 {
		eb.loadResult |= ResultMixedNewline // unified to the most used one
	}

	// Set newline type
	eb.NewlineType = []NewlineType{NewlineTypeLF, NewlineTypeCRLF, NewlineTypeCR}[utils.MaxValueIndex([]int{countLF, countCRLF, countCR})]

	// dump
	/*
		lines := ff.bows
		for i := 0; i < lines.Length(); i++ {
			s, _ := lines.String(i)
			//panic(s)
			// gelog.Info("%d %s", i, s)
		}
	*/
	// m.rows.Dump()
	return nil
}

// 1byte ずつ処理、効率が悪い
func ReadLine(r *bufio.Reader) ([]byte, NewlineType, error) {
	var line []byte

	for {
		b, err := r.ReadByte()
		if err != nil {
			// If only a newline, the size is zero.
			/*
				if err == io.EOF {
					// EOF immediately after the last byte means that the
					// final line has no newline.
					if len(line) > 0 {
						return line, NewlineTypeNone, nil
					}
				}
			*/
			return line, NewlineTypeNone, err
		}

		switch b {
		case '\n': // LF
			return line, NewlineTypeLF, nil

		case '\r': // CR
			// CR may be either CR or the first byte of CRLF.
			next, err := r.Peek(1)
			if err == nil && next[0] == '\n' {
				_, _ = r.ReadByte()
				return line, NewlineTypeCRLF, nil
			}

			return line, NewlineTypeCR, nil

		default:
			line = append(line, b)
		}
	}
}

// SetLangMode replaces the buffer's lang.Mode and notifies the listeners
// registered with AddLangModeListener (the syntax package uses this to
// switch the buffer's parser). Call it on the UI goroutine.
func (eb *EditBuffer) SetLangMode(langMode *lang.Mode) {
	eb.langMode = langMode
	eb.emitLangMode()
}

func (eb *EditBuffer) GetLangMode() *lang.Mode {
	return eb.langMode
}

// Bytes returns the contents of the edit buffer as a single byte slice.
// Line separators are joined using LF.
// The final row is included without appending a trailing newline.
//
// The returned startBytePos contains the starting byte position of each row
// in the resulting byte slice, followed by the total byte length.
func (eb *EditBuffer) Bytes() ([]byte, []int, error) {
	return utils.JoinRows(
		eb.rows.BytesArray(),
		NewlineTypeLF.Bytes(),
		true,
	)
}

func (eb *EditBuffer) Save() (Result, error) {
	result := ResultNone

	if eb.codec.lossy {
		return result, ErrLossy
	}

	if (*eb.langMode).IsFormattingBeforeSave() {
		sourceBytes, _, err := eb.Bytes()
		if err != nil {
			return result, err
		}

		formatted, err := (*eb.langMode).Formatting(sourceBytes)
		if err == nil {
			formattedRows := bytes.Split(
				formatted,
				NewlineTypeLF.Bytes(),
			)
			eb.SetBytesArray(formattedRows)

			result |= ResultFormatted
		}
	}

	// The rows are UTF-8 and joined with LF; the file gets its own newline
	// and encoding back. The conversion is done before the file is touched,
	// so a character that the encoding cannot hold leaves the file as it is.
	text, _, err := eb.Bytes()
	if err != nil {
		return result, err
	}
	sourceBytes, err := charset.Encode(text, eb.codec.spec, eb.codec.macNFD, eb.NewlineType.Bytes())
	if err != nil {
		return result, err
	}

	// permission は後回し
	if err := os.WriteFile(eb.path, sourceBytes, 0644); err != nil {
		return result, err
	}

	eb.metaDirty = false
	eb.recordDisk() // our own write is not an external change
	eb.emitSaved()
	return result | ResultSaved, nil
}

// Backup copies the file on disk to a backup file next to it, named
// "<path>.~YYYYMMDD_HHMMSS-NNN~" (the time of the backup, and NNN counting
// from 001 when that name is taken). It fails when the file does not exist.
func (eb *EditBuffer) Backup() error {
	if !utils.ExistsFile(eb.path) {
		return fmt.Errorf("(No file that need to be backup)")
	}

	/* for i := 1; i < 1_000_000; i++ {
		backup := fmt.Sprintf("%s.~%d~", eb.path, i)
		if !utils.ExistsFile(backup) {
			return utils.CopyFile(eb.path, backup)
		}
	}
	return fmt.Errorf("Too many backups") */

	nowStr := time.Now().Format("20060102_150405")
	for i := 1; i < 1000; i++ {
		backup := fmt.Sprintf("%s.~%s-%03d~", eb.path, nowStr, i)
		if !utils.ExistsFile(backup) {
			return utils.CopyFile(eb.path, backup)
		}
	}
	return fmt.Errorf("Too many backups")
}

// Setter/Getter

func (eb *EditBuffer) SetPath(path string) {
	eb.path = path
	eb.base = filepath.Base(path)
	eb.ext = filepath.Ext(path)
	eb.dispPath = eb.base
	// The state recorded for the old path means nothing for the new one (an
	// existing file was already confirmed to be overwritten by Save as).
	eb.recordDisk()
}

func (eb *EditBuffer) GetPath() string {
	return eb.path
}

func (eb *EditBuffer) GetBase() string {
	return eb.base
}

func (eb *EditBuffer) GetDispPath() string {
	return eb.dispPath
}

/* func (eb *EditBuffer) GetClass() string {
	return eb.ext
} */

func (eb *EditBuffer) GetNewLine() NewlineType {
	return eb.NewlineType
}

func (eb *EditBuffer) GetTabWidth() int {
	return (*eb.langMode).GetTabWidth()
}

// Flags

func (eb *EditBuffer) SetReadonly(b bool) {
	if b {
		eb.flags |= READONLY
	} else {
		eb.flags &= ^READONLY
	}
}

// IsReadonly reports whether the buffer is read-only: the file is, or it
// cannot be written back without loss (see IsLossy).
func (eb *EditBuffer) IsReadonly() bool {
	return eb.flags&READONLY != 0 || eb.codec.lossy
}

func (eb *EditBuffer) IsDirtyFlag() bool {
	return eb.UndoAction.IsDirty() || eb.metaDirty
}
