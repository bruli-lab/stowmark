package snapshot

type FailedCheck struct {
	path, reason string
}

func (f FailedCheck) Path() string {
	return f.path
}

func (f FailedCheck) Reason() string {
	return f.reason
}

func NewFailedCheck(path, reason string) *FailedCheck {
	return &FailedCheck{path: path, reason: reason}
}

type VerifyResult struct {
	snapshotID string
	totalFiles int
	failed     []FailedCheck
}

func (v *VerifyResult) SnapshotID() string {
	return v.snapshotID
}

func (v *VerifyResult) TotalFiles() int {
	return v.totalFiles
}

func (v *VerifyResult) Failed() []FailedCheck {
	return v.failed
}

func (v *VerifyResult) AddFailed(failed FailedCheck) {
	v.failed = append(v.failed, failed)
	v.totalFiles++
}

func (v *VerifyResult) AddSuccess() {
	v.totalFiles++
}

func (v *VerifyResult) IsSuccess() bool {
	return len(v.failed) == 0
}

func NewVerifyResult(snapshotID string) *VerifyResult {
	return &VerifyResult{snapshotID: snapshotID}
}
