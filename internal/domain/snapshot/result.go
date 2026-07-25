package snapshot

type FailedResult struct {
	path, reason string
}

func (f FailedResult) Path() string {
	return f.path
}

func (f FailedResult) Reason() string {
	return f.reason
}

func NewFailedResult(path, reason string) *FailedResult {
	return &FailedResult{path: path, reason: reason}
}

type Result struct {
	snapshotID string
	totalFiles int
	failed     []FailedResult
}

func (v *Result) SnapshotID() string {
	return v.snapshotID
}

func (v *Result) TotalFiles() int {
	return v.totalFiles
}

func (v *Result) Failed() []FailedResult {
	return v.failed
}

func (v *Result) AddFailed(failed FailedResult) {
	v.failed = append(v.failed, failed)
	v.totalFiles++
}

func (v *Result) AddSuccess() {
	v.totalFiles++
}

func (v *Result) IsSuccess() bool {
	return len(v.failed) == 0
}

func NewResult(snapshotID string) *Result {
	return &Result{snapshotID: snapshotID}
}
