package snapshot

type File struct {
	path string
	size int64
	hash string
}

func (f *File) Path() string {
	return f.path
}

func (f *File) Size() int64 {
	return f.size
}

func (f *File) Hash() string {
	return f.hash
}

func (f *File) AddHash(hash string) {
	f.hash = hash
}

func NewFile(path string, size int64) *File {
	return &File{path: path, size: size}
}
