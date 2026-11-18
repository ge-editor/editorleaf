// Package charset converts a file between the bytes on disk and the text the
// edit buffer holds.
//
// The buffer always holds UTF-8. Open reads the file: Prescan finds out the
// encoding (BOM, UTF-16, ISO-2022-JP, UTF-8, CP932, EUC-JP), Decode converts
// to UTF-8, and a file that is NFD as a whole (macOS, "UTF-8-mac") is
// normalized to NFC. Encode does it the other way round, so the file is
// written in the encoding it was read in.
//
// Every conversion is checked by converting back: a file that does not come
// back byte for byte is reported as Lossy, and the buffer must not write it.
// The newlines are not handled here (see editbuffer): Encode only replaces
// the LF of the text with the newline the file has.
package charset

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	xunicode "golang.org/x/text/encoding/unicode"
)

// Encoding is a character encoding of a file.
type Encoding int

const (
	UTF8 Encoding = iota // the zero value: the default of a new file
	UTF16LE
	UTF16BE
	CP932 // Shift_JIS of Windows (Windows-31J)
	EUCJP
	ISO2022JP
)

// Spec is an encoding with its byte order mark. A BOM only exists for UTF-8
// and UTF-16.
type Spec struct {
	Enc Encoding
	BOM bool
}

func (e Encoding) String() string {
	switch e {
	case UTF8:
		return "UTF-8"
	case UTF16LE:
		return "UTF-16LE"
	case UTF16BE:
		return "UTF-16BE"
	case CP932:
		return "CP932"
	case EUCJP:
		return "EUC-JP"
	case ISO2022JP:
		return "ISO-2022-JP"
	}
	return fmt.Sprintf("Encoding(%d)", int(e))
}

func (s Spec) String() string {
	if s.BOM {
		return s.Enc.String() + "-BOM"
	}
	return s.Enc.String()
}

var specs = []Spec{
	{UTF8, false}, {UTF8, true},
	{UTF16LE, false}, {UTF16LE, true},
	{UTF16BE, false}, {UTF16BE, true},
	{CP932, false}, {EUCJP, false}, {ISO2022JP, false},
}

// SpecNames returns the names ParseSpec accepts first, for completion.
func SpecNames() []string {
	names := make([]string, len(specs))
	for i, s := range specs {
		names[i] = s.String()
	}
	return names
}

// ParseSpec reads the name of an encoding. The case, "-", "_" and spaces do
// not matter: "UTF-8-BOM", "utf8bom", "sjis", "Shift_JIS", "euc-jp".
func ParseSpec(name string) (Spec, bool) {
	key := strings.ToLower(name)
	key = strings.NewReplacer("-", "", "_", "", " ", "").Replace(key)
	switch key {
	case "utf8":
		return Spec{UTF8, false}, true
	case "utf8bom":
		return Spec{UTF8, true}, true
	case "utf16le":
		return Spec{UTF16LE, false}, true
	case "utf16", "utf16lebom":
		return Spec{UTF16LE, true}, true
	case "utf16be":
		return Spec{UTF16BE, false}, true
	case "utf16bebom":
		return Spec{UTF16BE, true}, true
	case "cp932", "ms932", "windows31j", "sjis", "shiftjis":
		return Spec{CP932, false}, true
	case "eucjp":
		return Spec{EUCJP, false}, true
	case "iso2022jp", "jis":
		return Spec{ISO2022JP, false}, true
	}
	return Spec{}, false
}

var (
	bomUTF8    = []byte{0xEF, 0xBB, 0xBF}
	bomUTF16LE = []byte{0xFF, 0xFE}
	bomUTF16BE = []byte{0xFE, 0xFF}
)

func (s Spec) bomBytes() []byte {
	if !s.BOM {
		return nil
	}
	switch s.Enc {
	case UTF8:
		return bomUTF8
	case UTF16LE:
		return bomUTF16LE
	case UTF16BE:
		return bomUTF16BE
	}
	return nil
}

// xenc is the x/text encoding of e, nil for UTF-8 and CP932 (handled here).
func xenc(e Encoding) encoding.Encoding {
	switch e {
	case UTF16LE:
		return xunicode.UTF16(xunicode.LittleEndian, xunicode.IgnoreBOM)
	case UTF16BE:
		return xunicode.UTF16(xunicode.BigEndian, xunicode.IgnoreBOM)
	case EUCJP:
		return japanese.EUCJP
	case ISO2022JP:
		return japanese.ISO2022JP
	}
	return nil
}

// ---------------------------------------------------------------------------
// Decode
// ---------------------------------------------------------------------------

// Decode converts the bytes of a file to UTF-8, reading them as spec says.
// lossless is false when they cannot be read that way, or when the text does
// not encode back to the very same bytes; the text then has U+FFFD for what
// could not be read.
func Decode(raw []byte, spec Spec) (text []byte, lossless bool) {
	body := raw
	if bom := spec.bomBytes(); bom != nil && bytes.HasPrefix(raw, bom) {
		body = raw[len(bom):]
	}

	switch spec.Enc {
	case UTF8:
		if utf8.Valid(body) {
			return body, true
		}
		return []byte(strings.ToValidUTF8(string(body), "\uFFFD")), false

	case CP932:
		text, ok := cp932Decode(body)
		if !ok {
			return text, false
		}
		back, err := cp932Encode(text)
		return text, err == nil && bytes.Equal(back, body)

	default:
		x := xenc(spec.Enc)
		text, err := x.NewDecoder().Bytes(body)
		if err != nil {
			return []byte(strings.ToValidUTF8(string(text), "\uFFFD")), false
		}
		back, err := x.NewEncoder().Bytes(text)
		return text, err == nil && bytes.Equal(back, body)
	}
}

// ---------------------------------------------------------------------------
// Encode
// ---------------------------------------------------------------------------

// EncodeError is a character that the encoding of the file cannot hold.
type EncodeError struct {
	Line int // 1-based line of the file
	Rune rune
	Spec Spec
}

func (e *EncodeError) Error() string {
	return fmt.Sprintf("line %d: U+%04X (%c) cannot be written as %s", e.Line, e.Rune, e.Rune, e.Spec)
}

// Encode converts text (UTF-8, rows joined with LF) to the bytes of a file:
// NFD again if the file was read as NFD (macNFD), the LF replaced by nl, then
// the encoding with its BOM. A character that the encoding cannot hold is an
// *EncodeError; nothing is replaced silently.
func Encode(text []byte, spec Spec, macNFD bool, nl []byte) ([]byte, error) {
	if macNFD && spec.Enc == UTF8 {
		text = ToNFD(text)
	}
	if len(nl) > 0 && !bytes.Equal(nl, []byte{'\n'}) {
		text = bytes.ReplaceAll(text, []byte{'\n'}, nl)
	} else {
		nl = []byte{'\n'}
	}

	out, off, r, err := encodeBody(text, spec.Enc)
	if err != nil {
		return nil, &EncodeError{
			Line: bytes.Count(text[:min(off, len(text))], nl) + 1,
			Rune: r,
			Spec: spec,
		}
	}
	if bom := spec.bomBytes(); bom != nil {
		out = append(append(make([]byte, 0, len(bom)+len(out)), bom...), out...)
	}
	return out, nil
}

// encodeBody encodes without a BOM. On failure it returns the offset and the
// rune that cannot be encoded.
func encodeBody(text []byte, e Encoding) (out []byte, off int, r rune, err error) {
	switch e {
	case UTF8:
		return text, 0, 0, nil
	case CP932:
		out, err := cp932Encode(text)
		if err != nil {
			ee := err.(*cp932Error)
			return nil, ee.offset, ee.r, err
		}
		return out, 0, 0, nil
	}

	x := xenc(e)
	out, err = x.NewEncoder().Bytes(text)
	if err == nil {
		return out, 0, 0, nil
	}
	// Find the character: encode one by one (only when it failed).
	for i := 0; i < len(text); {
		c, n := utf8.DecodeRune(text[i:])
		if c >= 0x80 {
			if _, e1 := x.NewEncoder().Bytes(text[i : i+n]); e1 != nil {
				return nil, i, c, err
			}
		}
		i += n
	}
	return nil, 0, utf8.RuneError, err
}

// ---------------------------------------------------------------------------
// File
// ---------------------------------------------------------------------------

// File is a file read for the buffer.
type File struct {
	Text   []byte // UTF-8, the newlines are those of the file
	Spec   Spec   // the encoding it was read in; Encode writes it back
	MacNFD bool   // it was NFD and Text is NFC; Encode makes it NFD again
	Binary bool   // it looks like binary data
	Lossy  bool   // it cannot be written back as it was read
}

// Open reads the bytes of a file: Prescan, Decode, and the NFC conversion of
// a file that is NFD as a whole.
func Open(raw []byte) File {
	info := Prescan(raw)
	return open(raw, info.Spec, info.Binary, info.Lossy)
}

// OpenAs is Open for an encoding that the user chose (ESC x reload-encoding).
func OpenAs(raw []byte, spec Spec) File {
	return open(raw, spec, false, false)
}

func open(raw []byte, spec Spec, binary, lossy bool) File {
	text, ok := Decode(raw, spec)
	f := File{Text: text, Spec: spec, Binary: binary, Lossy: lossy || !ok}
	if bytes.IndexByte(text, 0) >= 0 {
		// NUL is not text (UTF-32, executables, ...).
		f.Binary = true
		f.Lossy = true
	}
	if spec.Enc == UTF8 && !f.Lossy && IsMacNFD(text) {
		f.Text = ToNFC(text)
		f.MacNFD = true
	}
	return f
}

// ReadFile reads and Opens a file. For the code that looks at files that are
// not open in a buffer (rename, the text of the marks): the text is what the
// buffer would have.
func ReadFile(path string) (File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	return Open(raw), nil
}
