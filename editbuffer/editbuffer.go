package editbuffer

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ge-editor/editorleaf/editbuffer/rows"
	"github.com/ge-editor/gecore/define"
	"github.com/ge-editor/gecore/lang"
	"github.com/ge-editor/gecore/screen"
	"github.com/ge-editor/utils"
)

type NewlineType int

func (n NewlineType) String() string {
	switch n {
	case NewlineTypeLF:
		return "LF"
	case NewlineTypeCRLF:
		return "CRLF"
	case NewlineTypeCR:
		return "CR"
	default:
		return "UNKNOWN"
	}
}

const (
	NewlineTypeNo NewlineType = iota
	NewlineTypeLF
	NewlineTypeCRLF
	NewlineTypeCR
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

	*rows.RowsStruct
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

		RowsStruct:  nil,
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

// No undo/redo functionality
func (eb *EditBuffer) RemoveRegion(cursor1, cursor2 screen.Cursor) *[]byte {
	return eb.removeRegion(cursor1, cursor2, true)
}

// No undo/redo functionality
func (eb *EditBuffer) GetRegion(cursor1, cursor2 screen.Cursor) *[]byte {
	return eb.removeRegion(cursor1, cursor2, false)
}

// Not within undo/redo functionality
func (eb *EditBuffer) removeRegion(start, end screen.Cursor, doRemove bool) *[]byte {
	// Checked row index. The start position of the region is after the end position, or the end position is beyond the last line
	if start.RowIndex > end.RowIndex || end.RowIndex >= eb.RowsLength() {
		return nil
	}
	if start.RowIndex == end.RowIndex && start.ColIndex >= end.ColIndex {
		return nil
	}

	topRow := eb.Rows().Row(start.RowIndex)
	// Checked col index. The start position of the region is the right of newline or EOF
	if start.ColIndex >= topRow.Length() {
		return nil
	}

	bottomRow := eb.Rows().Row(end.RowIndex)
	// Checked col index. The end position of the region is the right of newline or EOF
	if end.ColIndex >= bottomRow.Length() {
		return nil
	}

	if start.RowIndex == end.RowIndex {
		removed := topRow.SubBytes(start.ColIndex, end.ColIndex)
		if doRemove {
			*topRow = topRow.Delete(start.ColIndex, end.ColIndex)
		}
		return &removed
	}

	// Compute cap and allocate
	removed := make([]byte, 0, func() int {
		cap := topRow.Length() - start.ColIndex // top row
		for i := start.RowIndex + 1; i < end.RowIndex; i++ {
			cap += eb.Rows().Row(i).Length() // middle row
		}
		cap += end.ColIndex // bottom row byte size
		return cap
	}())

	// top row
	removed = append(removed, (*topRow)[start.ColIndex:]...)
	if doRemove {
		*topRow = (*topRow)[:start.ColIndex]
	}
	// middle rows
	for i := start.RowIndex + 1; i < end.RowIndex; i++ {
		removed = append(removed, eb.Rows().Row(i).Bytes()...)
	}
	// bottom row
	removed = append(removed, (*bottomRow)[:end.ColIndex]...)
	// Remove middle and bottom rows
	if doRemove {
		*topRow = append(*topRow, (*bottomRow)[end.ColIndex:]...)
		if end.RowIndex-start.RowIndex > 0 {
			eb.Rows().Delete(start.RowIndex+1, end.RowIndex+1)
		}
	}
	return &removed
}

// Split s []rune by sep rune
// sep is not deleted
func Split(s []rune, sep rune) (r [][]rune) {
	for {
		if len(s) == 0 {
			break
		}

		i := slices.Index(s, sep)
		if i == -1 {
			r = append(r, s)
			break
		}

		i += 1 // including separator
		r = append(r, s[:i])
		s = s[i:]
	}
	return
}

// SplitByLF split s []byte by lf
// lf is not deleted
func SplitByLF(s []byte) (results [][]byte) {
	for {
		if len(s) == 0 {
			break
		}

		i := slices.Index(s, '\n')
		if i == -1 {
			results = append(results, s)
			break
		}

		i += 1 // including separator
		results = append(results, s[:i])
		s = s[i:]
	}
	return
}

// New file
func (eb *EditBuffer) New() error {
	eb.RowsStruct = rows.New()
	eb.Rows().Add([]byte{define.EOF})

	// Set newline type
	eb.NewlineType = NewlineTypeLF
	// m.rows.Dump()
	return nil
}

// Load file
func (eb *EditBuffer) Load() error {
	/*
		info, err := prescan.Analyze(eb.path)
		if err != nil {
			return err
		}

		eb.encoding = info.Encoding
		eb.newline = convertNewline(info.Newline)

		fp.Seek(0, io.SeekStart)
	*/

	/*
		encoding, err := (*encorder).GuessCharset(ff.path, 128)
		if err != nil {
			return err
		}
		ff.encoding = encoding
	*/

	fp, err := os.Open(eb.path)
	if err != nil {
		return err
	}
	defer fp.Close()

	reader := bufio.NewReader(fp)

	/* 	scanLines := newScanLines(eb.encoding)
	   	scanner := bufio.NewScanner(fp)
	   	scanner.Split(scanLines.scanLines)
	*/
	// ff.rows__ = NewRows()
	//
	// ff.RowsStruct.New()
	eb.RowsStruct = rows.New()
	var countLF, countCRLF, countCR int
	for {
		line, nlType, err := readLine(reader)

		switch nlType {
		case NewlineTypeLF:
			countLF++
		case NewlineTypeCRLF:
			countCRLF++
		case NewlineTypeCR:
			countCR++
		}

		if len(line) > 0 {
			// b := make([]byte, len(line))
			b := make([]byte, len(line), len(line)+16)
			copy(b, line)
			eb.Rows().Add(b)
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	// var row *Row__
	// if the file size is zero
	// if ff.LenRows__() == 0 {
	if eb.RowsLength() == 0 {
		//row = NewRow()
		//ff.rows__.append(row)

		eb.Rows().Add([]byte{define.EOF})
	} else {
		// row = ff.Row__(ff.Rows__().LenRows__() - 1)
		//
		linesIndex := eb.RowsLength() - 1
		// lineIndex, _ := ff.rows.GetColLength(linesIndex)
		lineIndex := eb.Rows().Row(linesIndex).Length()
		if ch, _, _ := eb.Rows().Row(linesIndex).DecodeRune(lineIndex - 1); ch == '\n' {
			eb.Rows().Add([]byte{define.EOF})
		} else {
			eb.Rows().Row(linesIndex).Add([]byte{define.EOF})
		}
	}
	/*
		if row.LenCh__() > 0 && row.Ch__(row.LenCh__()-1) == '\n' {
			// Add EOF
			// if row.LenCh() > 0 && row.Ch(row.LenCh()-1) == '\n' {
			row = NewRow()
			ff.rows__.append(row)
		}
		row.append(define.EOF)
	*/
	// gelog.Info("%d,%d", linesIndex, lineIndex-1)

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

func readLine(r *bufio.Reader) ([]byte, NewlineType, error) {
	line, err := r.ReadBytes('\n')

	if err != nil && err != io.EOF {
		return nil, 0, err
	}

	var newlineType NewlineType

	if len(line) > 0 {
		if bytes.HasSuffix(line, []byte("\r\n")) {
			newlineType = NewlineTypeCRLF
			line = append(line[:len(line)-2], '\n')
		} else if bytes.HasSuffix(line, []byte("\n")) {
			newlineType = NewlineTypeLF
		} else if bytes.HasSuffix(line, []byte("\r")) {
			newlineType = NewlineTypeCR
			line = append(line[:len(line)-1], '\n')
		}
	}

	return line, newlineType, err
}

func (eb *EditBuffer) SetLangMode(langMode *lang.Mode) {
	eb.langMode = langMode
}

func (eb *EditBuffer) GetLangMode() *lang.Mode {
	return eb.langMode
}

/*
	 const (
		ErrFormatted gecore.ErrorCode = iota + 1
		// ErrNotFormatting
		// ErrFormattingFailed
		ErrSaved
		// ErrSavingFailed
		// ErrPermissionDenied

	)

// Return error is joined errors

	func (eb *EditBuffer) Save() (results error) {
		if (*eb.langMode).IsFormattingBeforeSave() {
			// Combine [][]byte into a single byte slice
			sourceBytes, _, err := utils.JoinBytes(eb.BytesArray())
			if err != nil {
				return err
			}
			// Remove EOF Mark
			sourceBytes = sourceBytes[:len(sourceBytes)-1]
			formatted, err := (*eb.langMode).Formatting(sourceBytes)
			if err == nil {
				// Add EOF Mark and split formatted source
				formattedRows := bytes.SplitAfter(append(formatted, define.EOF), []byte("\n"))
				eb.SetRows(formattedRows)
				results = errors.Join(results, gecore.NewGeError(ErrFormatted, "formatted"))
			}
		}

		var sb strings.Builder // Consider using strings.Builder for potential performance gains

		newline := []byte{'\n'} // Default to LF
		if eb.newline&CRLF > 0 {
			newline = []byte{'\r', '\n'}
		} else if eb.newline&CR > 0 {
			newline = []byte{'\r'}
		}

		lastRowIndex := eb.RowsLength() - 1
		for i, row := range *eb.Rows() {
			if row == nil {
				return errors.Join(results, fmt.Errorf("row is nothing"))
			}
			lineBufferLen := eb.Rows().Row(i).Length()
			if i == lastRowIndex && row[lineBufferLen-1] == define.EOF {
				// skip EOF mark
				sb.Write(row[:lineBufferLen-1])
				break
			} else if row[lineBufferLen-1] == define.LF {
				sb.Write(row[:lineBufferLen-1]) // skip newline and
				sb.Write(newline)              // append
			} else {
				sb.Write(row[:lineBufferLen])
			}
		}

		err := os.WriteFile(eb.path, []byte(sb.String()), 0644)
		if err == nil {
			err = gecore.NewGeError(ErrSaved, "saved")
		}
		return errors.Join(results, err)
	}
*/

func (eb *EditBuffer) Save() (Result, error) {
	result := ResultNone

	if (*eb.langMode).IsFormattingBeforeSave() {
		sourceBytes, _, err := utils.JoinBytes(eb.BytesArray())
		if err != nil {
			return result, err
		}

		// Remove EOF Mark
		sourceBytes = sourceBytes[:len(sourceBytes)-1]

		formatted, err := (*eb.langMode).Formatting(sourceBytes)
		if err == nil {
			// Add EOF Mark and split formatted source
			formattedRows := bytes.SplitAfter(
				append(formatted, define.EOF),
				[]byte("\n"),
			)
			eb.SetRows(formattedRows)

			result |= ResultFormatted
		}
	}

	var sb strings.Builder

	newline := []byte{'\n'}
	switch eb.NewlineType {
	case NewlineTypeCRLF:
		newline = []byte{'\r', '\n'}
	case NewlineTypeCR:
		newline = []byte{'\r'}
	}

	lastRowIndex := eb.RowsLength() - 1

	for i, row := range *eb.Rows() {
		if row == nil {
			// return errors.Join(result, ErrRowNil)
			return result, errors.New("row is nil")
		}

		lineBufferLen := eb.Rows().Row(i).Length()

		switch {
		case i == lastRowIndex && row[lineBufferLen-1] == define.EOF:
			// skip EOF mark
			sb.Write(row[:lineBufferLen-1])

		case row[lineBufferLen-1] == define.LF:
			// skip EOF mark
			sb.Write(row[:lineBufferLen-1])
			sb.Write(newline)

		default:
			sb.Write(row[:lineBufferLen])
		}
	}

	if err := os.WriteFile(eb.path, []byte(sb.String()), 0644); err != nil {
		return result, err
	}

	return result | ResultSaved, nil
	// return errors.Join(result, ErrSaved)
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

/*
	 func (ff *File) SetDirtyFlag(b bool) {
		if b {
			ff.flags |= dirty
		} else {
			ff.flags &= ^dirty
		}
	}
*/

func (eb *EditBuffer) IsDirtyFlag() bool {
	// return !ff.UndoAction.IsEmpty()
	return eb.UndoAction.IsDirty()
}
