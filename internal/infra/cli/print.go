package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
)

func printResult(output io. Writer, result *snapshot.Result) error {
	writer := tabwriter.NewWriter(
		output,
		0,
		4,
		2,
		' ',
		0,
	)

	status := "OK"
	if !result.IsSuccess() {
		status = "FAILED"
	}

	failedFiles := len(result.Failed())
	validFiles := result.TotalFiles() - failedFiles

	if _, err := fmt.Fprintf(
		writer,
		"SNAPSHOT:\t%s\n"+
			"STATUS:\t%s\n"+
			"RESTORED:\t%d\n"+
			"VALID:\t%d\n"+
			"INVALID:\t%d\n",
		result.SnapshotID(),
		status,
		result.TotalFiles(),
		validFiles,
		failedFiles,
	); err != nil {
		return err
	}

	if failedFiles == 0 {
		return writer.Flush()
	}

	if _, err := fmt.Fprintln(
		writer,
		"\nPATH\tSTATUS\tREASON",
	); err != nil {
		return err
	}

	for _, check := range result.Failed() {
		if _, err := fmt.Fprintf(
			writer,
			"%s\tINVALID\t%s\n",
			check.Path(),
			check.Reason(),
		); err != nil {
			return err
		}
	}

	return writer.Flush()
}
