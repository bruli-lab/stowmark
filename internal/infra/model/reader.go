package model

import (
	"context"
	"io"
)

type ContextReader struct {
	Ctx    context.Context
	Reader io.Reader
}

func (r ContextReader) Read(buffer []byte) (int, error) {
	if err := r.Ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(buffer)
}
