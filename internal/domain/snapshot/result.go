package snapshot

type Result struct {
	id        string
	fileCount int
	totalSize int64
}

func (r Result) Id() string {
	return r.id
}

func (r Result) FileCount() int {
	return r.fileCount
}

func (r Result) TotalSize() int64 {
	return r.totalSize
}

func NewResult(id string, fileCount int, totalSize int64) *Result {
	return &Result{id: id, fileCount: fileCount, totalSize: totalSize}
}
