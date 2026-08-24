package cli

import (
	"fmt"
	"io"
	"text/tabwriter"
)

type Failed struct {
	Path   string
	Reason string
}

func printResult(output io.Writer, snapshotID string, totalFiles int, failed []Failed, isSuccess bool) error {
	writer := tabwriter.NewWriter(
		output,
		0,
		4,
		2,
		' ',
		0,
	)

	status := "OK"
	if !isSuccess {
		status = "FAILED"
	}

	failedFiles := len(failed)
	validFiles := totalFiles - failedFiles

	if _, err := fmt.Fprintf(
		writer,
		"SNAPSHOT:\t%s\n"+
			"STATUS:\t%s\n"+
			"RESTORED:\t%d\n"+
			"VALID:\t%d\n"+
			"INVALID:\t%d\n",
		snapshotID,
		status,
		totalFiles,
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

	for _, check := range failed {
		if _, err := fmt.Fprintf(
			writer,
			"%s\tINVALID\t%s\n",
			check.Path,
			check.Reason,
		); err != nil {
			return err
		}
	}

	return writer.Flush()
}
