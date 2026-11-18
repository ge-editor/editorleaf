package editbuffer

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"time"

	"github.com/ge-editor/editorleaf/charset"
)

// The buffer holds UTF-8 text with LF between the rows, whatever the file is
// like. What the file is like is remembered here, and Save writes it back that
// way: the encoding with its BOM, NFD for a file that was NFD, the newline.
// The conversion is done by the charset package.

// ErrLossy is the reason a buffer cannot be saved: the file was not read
// without loss (not valid in any encoding that ge knows, or binary data), so
// writing the buffer would damage it.
var ErrLossy = errors.New("not saved: the file could not be read without loss " +
	"(ESC x reload-encoding NAME reads it in another encoding)")

type fileCodec struct {
	spec   charset.Spec
	macNFD bool // the file was NFD (UTF-8-mac); the rows are NFC
	lossy  bool // see ErrLossy
	binary bool
}

// loadFile replaces the content with a file that charset has read.
func (eb *EditBuffer) loadFile(f charset.File) error {
	if err := eb.loadText(bytes.NewReader(f.Text)); err != nil {
		return err
	}
	eb.codec = fileCodec{spec: f.Spec, macNFD: f.MacNFD, lossy: f.Lossy, binary: f.Binary}
	eb.metaDirty = false

	switch f.Spec.Enc {
	case charset.UTF16LE, charset.UTF16BE:
		eb.loadResult |= ResultFromUTF16
	case charset.CP932:
		eb.loadResult |= ResultFromShiftJIS
	case charset.EUCJP:
		eb.loadResult |= ResultFromEUCJP
	case charset.ISO2022JP:
		eb.loadResult |= ResultFromISO2022JP
	}
	if f.MacNFD {
		eb.loadResult |= ResultNormalizedUTF8Mac
	}
	if f.Binary {
		eb.loadResult |= ResultBinary
	} else if f.Lossy {
		eb.loadResult |= ResultLossy
	}
	return nil
}

// replaceWith replaces the content of the buffer with a file that charset
// has read, forgets the undo history, and takes the encoding and the newline
// of the file. The listeners are notified like for any other replacement of
// the rows (SetBytesArray).
func (eb *EditBuffer) replaceWith(f charset.File, mt time.Time, size int64, ok bool) error {
	fresh := &EditBuffer{}
	if err := fresh.loadFile(f); err != nil {
		return err
	}

	eb.NewlineType = fresh.NewlineType
	eb.codec = fresh.codec
	eb.loadResult = fresh.loadResult
	eb.metaDirty = false
	eb.SetBytesArray(fresh.rows.BytesArray())
	eb.UndoAction.Clear()
	eb.setDisk(mt, size, ok)

	if info, err := os.Stat(eb.path); err == nil {
		eb.SetReadonly(info.Mode().Perm()&fs.FileMode(0200) == 0)
	}
	return nil
}

// ReloadAs reads the file again, as the encoding says. It is for a file that
// Prescan took for another encoding. The unsaved changes and the undo history
// are discarded, like Reload does. A file that cannot be read without loss in
// this encoding is shown anyway, but read-only (IsLossy).
func (eb *EditBuffer) ReloadAs(spec charset.Spec) error {
	mt, size, ok := statRegular(eb.path)

	raw, err := os.ReadFile(eb.path)
	if err != nil {
		return err
	}
	return eb.replaceWith(charset.OpenAs(raw, spec), mt, size, ok)
}

// GetEncoding is the name of the encoding for the mode line: "UTF-8",
// "UTF-8-BOM", "CP932", ... A file that was NFD has "-mac" after it, and "!"
// means that it cannot be written back (IsLossy).
func (eb *EditBuffer) GetEncoding() string {
	s := eb.codec.spec.String()
	if eb.codec.macNFD {
		s += "-mac"
	}
	if eb.codec.lossy {
		s += "!"
	}
	return s
}

// EncodingSpec is the encoding that Save writes.
func (eb *EditBuffer) EncodingSpec() charset.Spec { return eb.codec.spec }

// IsLossy reports whether the buffer cannot be saved because the file was
// not read without loss.
func (eb *EditBuffer) IsLossy() bool { return eb.codec.lossy }

// LoadResult tells what was done when the file was read (converted from an
// encoding, mixed newlines unified, read-only because of loss).
func (eb *EditBuffer) LoadResult() Result { return eb.loadResult }

// CheckEncoding reports whether the buffer could be written as spec: it is
// an *charset.EncodeError that names the first line with a character the
// encoding cannot hold.
func (eb *EditBuffer) CheckEncoding(spec charset.Spec) error {
	text, _, err := eb.Bytes()
	if err != nil {
		return err
	}
	_, err = charset.Encode(text, spec, false, eb.NewlineType.Bytes())
	return err
}

// SetEncoding sets the encoding that Save writes. The rows are not changed.
// The buffer is modified until it is saved, although nothing was edited.
func (eb *EditBuffer) SetEncoding(spec charset.Spec) {
	if spec == eb.codec.spec {
		return
	}
	eb.codec.spec = spec
	if spec.Enc != charset.UTF8 {
		eb.codec.macNFD = false // NFD is only for UTF-8
	}
	eb.metaDirty = true
}

// SetNewline sets the newline that Save writes. The rows are not changed.
// The buffer is modified until it is saved, although nothing was edited.
func (eb *EditBuffer) SetNewline(nt NewlineType) {
	if nt == NewlineTypeNone || nt == eb.NewlineType {
		return
	}
	eb.NewlineType = nt
	eb.metaDirty = true
}
