package compression

import (
	"fmt"
	"io"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

type WriterCloser struct {
	Writer io.Writer
	Closer func() error
}

type ReaderCloser struct {
	Reader io.Reader
	Closer func()
}

type Handler interface {
	Encode(destination io.Writer, level *int) (*WriterCloser, error)
	Decode(origin io.Reader) (*ReaderCloser, error)
}

type HandlersFactory struct {
	handlers map[repository.CompressionType]Handler
}

func (e *HandlersFactory) GetHandler(ct repository.CompressionType) (Handler, error) {
	enc, ok := e.handlers[ct]
	if !ok {
		return nil, fmt.Errorf("unsupported compression type %s", ct.String())
	}
	return enc, nil
}

func NewHandlersFactory() *HandlersFactory {
	return &HandlersFactory{handlers: map[repository.CompressionType]Handler{
		repository.NoneCompressionType: noneHandler{},
		repository.ZstdCompressionType: zstdHandler{},
		repository.GzipCompressionType: GzipHandler{},
		repository.Lz4CompressionType:  Lz4Handler{},
		repository.XzCompressionType:   XzHandler{},
	}}
}
