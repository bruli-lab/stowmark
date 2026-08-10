package disk

import (
	"compress/gzip"
	"fmt"
	"io"
	"time"
)

type GzipHandler struct{}

func (g GzipHandler) Encode(destination io.Writer, level *int) (*WriterCloser, error) {
	writer, err := gzip.NewWriterLevel(destination, *level)
	if err != nil {
		return nil, fmt.Errorf("create gzip writer: %w", err)
	}

	writer.ModTime = time.Time{}
	writer.Name = ""
	writer.Comment = ""
	writer.OS = 255
	return &WriterCloser{
		Writer: writer,
		Closer: writer.Close,
	}, nil
}

func (g GzipHandler) Decode(origin io.Reader) (*ReaderCloser, error) {
	reader, err := gzip.NewReader(origin)
	if err != nil {
		return nil, fmt.Errorf("create gzip reader: %w", err)
	}
	return &ReaderCloser{
		Reader: reader,
		Closer: func() {
			_ = reader.Close()
		},
	}, nil
}
