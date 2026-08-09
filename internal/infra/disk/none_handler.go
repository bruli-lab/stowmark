package disk

import "io"

type noneHandler struct{}

func (n noneHandler) Decode(origin io.Reader) (io.Reader, func(), error) {
	return origin, nil, nil
}

func (n noneHandler) Encode(destination io.Writer, _ *int) (io.Writer, error) {
	return destination, nil
}
