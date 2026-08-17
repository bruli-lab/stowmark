package repository

type FormatVersion int

func (c FormatVersion) Int() int {
	return int(c)
}

const (
	FormatVersionOne FormatVersion = iota + 1
	FormatVersionTwo
	CurrentFormatVersion = FormatVersionTwo
)

func ParseFormatVersion(value int) FormatVersion {
	version := FormatVersion(value)

	if version < FormatVersionOne || version > CurrentFormatVersion {
		version = CurrentFormatVersion
	}

	return version
}
