package compression

import (
	"fmt"
	"io"

	"github.com/ulikunitz/xz"
)

type XzHandler struct{}

func (x XzHandler) Encode(destination io.Writer, _ *int) (*WriterCloser, error) {
	writer, err := xz.NewWriter(destination)
	if err != nil {
		return nil, fmt.Errorf("create xz encoder: %w", err)
	}

	return &WriterCloser{
		Writer: writer,
		Closer: writer.Close,
	}, nil
}

func (x XzHandler) Decode(origin io.Reader) (*ReaderCloser, error) {
	reader, err := xz.NewReader(origin)
	if err != nil {
		return nil, fmt.Errorf("create xz decoder: %w", err)
	}
	return &ReaderCloser{
		Reader: reader,
		Closer: func() {},
	}, nil
}
