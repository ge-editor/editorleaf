package editbuffer

import "strings"

type Result uint32

const (
	ResultNone      Result = 0
	ResultSaved     Result = 1 << iota // "buffer saved"
	ResultFormatted                    // "buffer formatted"

	// Buffer state
	ResultNewFile    // "(New file)"
	ResultLoadedFile // "(Loaded)"

	// Encoding transforms
	ResultNormalizedUTF8Mac // "Normalized from UTF-8-mac to UTF-8"
	ResultFromShiftJIS      // "Encoded from ShiftJIS to UTF-8"
	ResultFromEUCJP         // "Encoded from EUC-JP to UTF-8"
)

func (r Result) String() string {
	if r == ResultNone {
		return ""
	}

	var parts []string

	if r&ResultSaved != 0 {
		parts = append(parts, "Saved")
	}
	if r&ResultFormatted != 0 {
		parts = append(parts, "Formatted")
	}

	if r&ResultNewFile != 0 {
		parts = append(parts, "(New file)")
	}
	if r&ResultLoadedFile != 0 {
		parts = append(parts, "(Loaded)")
	}

	if r&ResultNormalizedUTF8Mac != 0 {
		parts = append(parts, "UTF-8-mac → UTF-8")
	}
	if r&ResultFromShiftJIS != 0 {
		parts = append(parts, "ShiftJIS → UTF-8")
	}
	if r&ResultFromEUCJP != 0 {
		parts = append(parts, "EUC-JP → UTF-8")
	}

	return strings.Join(parts, " | ")
}

/* var (
	ErrSaved     = errors.New("buffer saved")
	ErrFormatted = errors.New("buffer formatted")
	ErrRowNil    = errors.New("row is nil")
)

// Custom error
var (
	// Buffer messages
	ErrorNewFile    = errors.New("(New file)")
	ErrorLoadedFile = errors.New("(Loaded)")

	// Encoded messages
	ErrMac      = errors.New("Normalized from UTF-8-mac to UTF-8")
	ErrShiftJis = errors.New("Encoded from ShiftJIS to UTF-8")
	ErrEucJp    = errors.New("Encoded from EUC-JP to UTF-8")
)
*/
