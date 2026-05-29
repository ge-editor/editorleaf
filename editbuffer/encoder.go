package editbuffer

type Encoder interface {
	Encoder(string, *[]byte) error // Save file
	GuessCharset(path string, bytes int) (string, error)
	Decoder(*[]byte) (string, error) // Load file
	IsDecodedMessage(error) bool
}

/*
func SetEncoder(e *Encoder) {
	encorder = e
}
*/
