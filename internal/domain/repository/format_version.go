package repository

import "fmt"

var ErrUnsupportedFormatVersion = fmt.Errorf("unsupported repository format version")

type FormatVersion int

func (c FormatVersion) Int() int {
	return int(c)
}

func (c FormatVersion) ContentAddressed() bool {
	return c >= FormatVersionThree
}

const (
	FormatVersionOne FormatVersion = iota + 1
	FormatVersionTwo
	FormatVersionThree
	DefaultFormatVersion = FormatVersionTwo
	LatestFormatVersion  = FormatVersionThree
)

func ParseFormatVersion(value int) (FormatVersion, error) {
	version := FormatVersion(value)

	if version < FormatVersionOne || version > LatestFormatVersion {
		return 0, fmt.Errorf("%w: %d (supported: %d-%d)", ErrUnsupportedFormatVersion, value, FormatVersionOne, LatestFormatVersion)
	}

	return version, nil
}

func ParseFormatVersionOrDefault(value int) (FormatVersion, error) {
	if value == 0 {
		return DefaultFormatVersion, nil
	}

	return ParseFormatVersion(value)
}
