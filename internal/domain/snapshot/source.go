package snapshot

type Source struct {
	absolutePath string
	files        []File
}

func (s Source) AbsolutePath() string {
	return s.absolutePath
}

func (s Source) Files() []File {
	return s.files
}

func NewSource(absolutePath string, files []File) *Source {
	return &Source{absolutePath: absolutePath, files: files}
}
