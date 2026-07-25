package snapshot

type CreateResult struct {
	id        string
	fileCount int
	totalSize int64
}

func (r CreateResult) Id() string {
	return r.id
}

func (r CreateResult) FileCount() int {
	return r.fileCount
}

func (r CreateResult) TotalSize() int64 {
	return r.totalSize
}

func NewCreateResult(id string, fileCount int, totalSize int64) *CreateResult {
	return &CreateResult{id: id, fileCount: fileCount, totalSize: totalSize}
}
