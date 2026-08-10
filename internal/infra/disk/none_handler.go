package disk

import "io"

type noneHandler struct{}

func (n noneHandler) Decode(origin io.Reader) (*ReaderCloser, error) {
	return &ReaderCloser{
		Reader: origin,
		Closer: nil,
	}, nil
}

func (n noneHandler) Encode(destination io.Writer, _ *int) (*WriterCloser, error) {
	return &WriterCloser{
		Writer: destination,
		Closer: nil,
	}, nil
}
