package disk

import (
	"fmt"
	"io"

	"github.com/pierrec/lz4/v4"
)

type Lz4Handler struct{}

func (l Lz4Handler) Encode(destination io.Writer, level *int) (*WriterCloser, error) {
	writer := lz4.NewWriter(destination)
	if level == nil {
		return nil, fmt.Errorf("level is required")
	}
	compressionLevel, err := lz4Level(*level)
	if err != nil {
		return nil, err
	}

	if err := writer.Apply(
		lz4.CompressionLevelOption(compressionLevel),
		lz4.ConcurrencyOption(1),
	); err != nil {
		return nil, fmt.Errorf("configure lz4 encoder: %w", err)
	}
	return &WriterCloser{
		Writer: writer,
		Closer: writer.Close,
	}, nil
}

func (l Lz4Handler) Decode(origin io.Reader) (*ReaderCloser, error) {
	reader := lz4.NewReader(origin)
	return &ReaderCloser{
		Reader: reader,
		Closer: func() {},
	}, nil
}

func lz4Level(level int) (lz4.CompressionLevel, error) {
	switch level {
	case 0:
		return lz4.Fast, nil
	case 1:
		return lz4.Level1, nil
	case 2:
		return lz4.Level2, nil
	case 3:
		return lz4.Level3, nil
	case 4:
		return lz4.Level4, nil
	case 5:
		return lz4.Level5, nil
	case 6:
		return lz4.Level6, nil
	case 7:
		return lz4.Level7, nil
	case 8:
		return lz4.Level8, nil
	case 9:
		return lz4.Level9, nil
	default:
		return 0, fmt.Errorf(
			"invalid lz4 compression level %d: expected 0 to 9",
			level,
		)
	}
}
