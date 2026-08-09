package disk

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

type compressionHandler interface {
	Encode(destination io.Writer, level *int) (*WriterCloser, error)
	Decode(origin io.Reader) (*ReaderCloser, error)
}

type compressionHandlersFactory struct {
	handlers map[repository.CompressionType]compressionHandler
}

func (e *compressionHandlersFactory) getHandler(ct repository.CompressionType) (compressionHandler, error) {
	enc, ok := e.handlers[ct]
	if !ok {
		return nil, fmt.Errorf("unsupported compression type %s", ct.String())
	}
	return enc, nil
}

func newCompressionHandlersFactory() *compressionHandlersFactory {
	return &compressionHandlersFactory{handlers: map[repository.CompressionType]compressionHandler{
		repository.NoneCompressionType: noneHandler{},
		repository.ZstdCompressionType: zstdHandler{},
	}}
}
