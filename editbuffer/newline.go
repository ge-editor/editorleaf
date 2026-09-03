package editbuffer

type NewlineType int

const (
	NewlineTypeNone NewlineType = iota
	NewlineTypeLF
	NewlineTypeCRLF
	NewlineTypeCR
)

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

func (n NewlineType) Bytes() []byte {
	switch n {
	case NewlineTypeLF:
		return []byte{'\n'}
	case NewlineTypeCRLF:
		return []byte{'\r', '\n'}
	case NewlineTypeCR:
		return []byte{'\r'}
	default:
		return nil
	}
}
