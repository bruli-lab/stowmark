package snapshot

const (
	SymlinkKind   = "symbolic link"
	DeviceKind    = "device"
	SocketKind    = "socket"
	NamedPipeKind = "named pipe"
	IrregularKind = "irregular file"
)

type SkippedFile struct {
	path string
	kind string
}

func (s SkippedFile) Path() string {
	return s.path
}

func (s SkippedFile) Kind() string {
	return s.kind
}

func NewSkippedFile(path, kind string) SkippedFile {
	return SkippedFile{path: path, kind: kind}
}

type Source struct {
	absolutePath string
	files        []File
	skipped      []SkippedFile
}

func (s Source) AbsolutePath() string {
	return s.absolutePath
}

func (s Source) Files() []File {
	return s.files
}

func (s Source) Skipped() []SkippedFile {
	return s.skipped
}

func NewSource(absolutePath string, files []File, skipped []SkippedFile) *Source {
	return &Source{absolutePath: absolutePath, files: files, skipped: skipped}
}
