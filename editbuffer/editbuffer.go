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

	"github.com/ge-editor/editorleaf/editbuffer/rows"
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

	langMode *lang.Mode

	*rows.Rows
	encoding string
	NewlineType

	flags // readonly

	UndoAction *UndoStack
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

		Rows:        nil,
		encoding:    "UTF-8",
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
		if utils.SameFile(wd, dir) {
			eb.dispPath = filepath.Join(dir, eb.dispPath)
		}
	}

	eb.ext = filepath.Ext(eb.path)
}

func (eb *EditBuffer) ChangePath(path string) {
	eb.rawPath = path
	eb.init()
}

// New file
func (eb *EditBuffer) New() error {
	eb.Rows = rows.New()
	eb.Rows.AddRow([]byte{})

	eb.NewlineType = NewlineTypeLF
	return nil
}

// Load file
func (eb *EditBuffer) Load() error {
	fp, err := os.Open(eb.path)
	if err != nil {
		return err
	}
	defer fp.Close()

	reader := bufio.NewReader(fp)

	eb.Rows = rows.New()
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
		eb.AddRow(b)
		// }

		if err == io.EOF {
			finalRowNewlineType = newlineType
			break
		}
		if err != nil {
			return err
		}
	}

	if eb.Length() == 0 || finalRowNewlineType != NewlineTypeNone {
		eb.AddRow([]byte{})
	} /*  else {
		linesIndex := eb.RowsLength() - 1
		lineIndex := eb.Rows().Row(linesIndex).Length()
		if ch, _, _ := eb.Rows().Row(linesIndex).DecodeRune(lineIndex - 1); ch == '\n' {
			eb.Rows().Add([]byte{define.EOF})
		} else {
			eb.Rows().Row(linesIndex).Add([]byte{define.EOF})
		}
	} */

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

func (eb *EditBuffer) SetLangMode(langMode *lang.Mode) {
	eb.langMode = langMode
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
		eb.BytesArray(),
		NewlineTypeLF.Bytes(),
		true,
	)
}

func (eb *EditBuffer) Save() (Result, error) {
	result := ResultNone

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

	sourceBytes, _, err := utils.JoinRows(eb.BytesArray(), eb.GetNewLine().Bytes(), true)
	if err != nil {
		return result, err
	}

	// permission は後回し
	if err := os.WriteFile(eb.path, sourceBytes, 0644); err != nil {
		return result, err
	}

	return result | ResultSaved, nil
}

// would like to consider other formats such as dates.
func (eb *EditBuffer) Backup() error {
	if !utils.ExistsFile(eb.path) {
		return fmt.Errorf("(No file that need to be backup)")
	}

	for i := 1; i < 1_000_000; i++ {
		backup := fmt.Sprintf("%s.~%d~", eb.path, i)
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

func (eb *EditBuffer) GetClass() string {
	return eb.ext
}

func (eb *EditBuffer) GetEncoding() string {
	return eb.encoding
}

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

func (eb *EditBuffer) IsReadonly() bool {
	return eb.flags&READONLY != 0
}

func (eb *EditBuffer) IsDirtyFlag() bool {
	return eb.UndoAction.IsDirty()
}
