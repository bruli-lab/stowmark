package repository

import "errors"

const (
	NoneCompressionType CompressionType = "none"
	ZstdCompressionType CompressionType = "zstd"
	Lz4CompressionType  CompressionType = "lz4"
	XzCompressionType   CompressionType = "xz"
	GzipCompressionType CompressionType = "gzip"

	DefaultZstdLevel int = 3
	MinimumZstdLevel int = 1
	MaximumZstdLevel int = 22
)

var (
	compressionTypes = map[string]CompressionType{
		"none": NoneCompressionType,
		"zstd": ZstdCompressionType,
		"lz4":  Lz4CompressionType,
		"xz":   XzCompressionType,
		"gzip": GzipCompressionType,
	}

	ErrInvalidCompressionType = errors.New("invalid compression type")
	ErrInvalidZstdLevel       = errors.New("zstd compression level must be between 1 and 22")
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

func (c *Compression) CompType() CompressionType {
	return c.compType
}

func (c *Compression) Level() *int {
	return c.level
}

func (c *Compression) validate() error {
	switch c.compType {
	case ZstdCompressionType:
		if c.level == nil {
			c.level = new(DefaultZstdLevel)
		}
		if *c.level < MinimumZstdLevel || *c.level > MaximumZstdLevel {
			return ErrInvalidZstdLevel
		}
	case NoneCompressionType:
		c.level = nil
	}
	return nil
}

func NewCompression(compType CompressionType, level *int) (*Compression, error) {
	comp := Compression{compType: compType, level: level}
	if err := comp.validate(); err != nil {
		return nil, err
	}
	return &comp, nil
}
