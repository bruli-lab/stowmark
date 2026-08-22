package model

import "io"

type ReadCloser struct {
	io.Reader
	Closer io.Closer
}

func (r *ReadCloser) Close() error {
	return r.Closer.Close()
}
