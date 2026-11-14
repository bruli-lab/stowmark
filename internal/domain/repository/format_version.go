package repository

import "fmt"

var ErrUnsupportedFormatVersion = fmt.Errorf("unsupported repository format version")

type FormatVersion int

func (c FormatVersion) Int() int {
	return int(c)
}

const (
	FormatVersionOne FormatVersion = iota + 1
	FormatVersionTwo
	CurrentFormatVersion = FormatVersionTwo
)

func ParseFormatVersion(value int) (FormatVersion, error) {
	version := FormatVersion(value)

	if version < FormatVersionOne || version > CurrentFormatVersion {
		return 0, fmt.Errorf("%w: %d (supported: %d-%d)", ErrUnsupportedFormatVersion, value, FormatVersionOne, CurrentFormatVersion)
	}

	return version, nil
}

func ParseFormatVersionOrCurrent(value int) (FormatVersion, error) {
	if value == 0 {
		return CurrentFormatVersion, nil
	}

	return ParseFormatVersion(value)
}
