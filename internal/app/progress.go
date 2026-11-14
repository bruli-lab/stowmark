package app

import "github.com/bruli-lab/stowmark/internal/domain/snapshot"

type ProgressStage string

const (
	ProgressBackingUp   ProgressStage = "Backing up"
	ProgressRestoring   ProgressStage = "Restoring"
	ProgressVerifying   ProgressStage = "Verifying"
	ProgressReadingData ProgressStage = "Reading data"
	ProgressRekeying    ProgressStage = "Rekeying"
)

type Progress interface {
	Start(stage ProgressStage, items int, bytes int64)
	Advance(items int, bytes int64)
	Finish()
}

type noProgress struct{}

func (noProgress) Start(ProgressStage, int, int64) {}

func (noProgress) Advance(int, int64) {}

func (noProgress) Finish() {}

func progressOrNone(progress Progress) Progress {
	if progress == nil {
		return noProgress{}
	}
	return progress
}

func startProgress(progress Progress, stage ProgressStage, items int, bytes int64) Progress {
	reporter := progressOrNone(progress)
	reporter.Start(stage, items, bytes)
	return reporter
}

func contentTotals(files []snapshot.File) (items int, bytes int64) {
	for _, file := range files {
		bytes += contentSize(file)
	}
	return len(files), bytes
}

func contentSize(file snapshot.File) int64 {
	if !file.HasContent() {
		return 0
	}
	return file.Size()
}
