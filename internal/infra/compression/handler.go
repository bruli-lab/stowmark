package compression

import (
	"errors"
	"fmt"
	"io"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

var (
	ErrUnsupportedCompressionType = errors.New("unsupported compression type")
	ErrLevelRequired              = errors.New("compression level is required")
	ErrInvalidLevel               = errors.New("invalid compression level")
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
		return nil, fmt.Errorf("%w %s", ErrUnsupportedCompressionType, ct.String())
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
