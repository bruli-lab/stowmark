package repository

import "errors"

const NoneCompressionType CompressionType = "none"

var (
	compressionTypes = map[string]CompressionType{
		"none": NoneCompressionType,
	}

	ErrInvalidCompressionType = errors.New("invalid compression type")
)

type CompressionType string

func (c CompressionType) String() string {
	return string(c)
}

func ParseCompressionType(s string) (*CompressionType, error) {
	ct, ok := compressionTypes[s]
	if !ok {
		return nil, ErrInvalidCompressionType
	}
	return &ct, nil
}

type Compression struct {
	compType CompressionType
	level    *int
}

func (c Compression) CompType() CompressionType {
	return c.compType
}

func (c Compression) Level() *int {
	return c.level
}

func NewCompression(compType CompressionType, level *int) *Compression {
	return &Compression{compType: compType, level: level}
}

func NoneCompression() *Compression {
	return NewCompression(NoneCompressionType, nil)
}
