package cli

import (
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/spf13/cobra"
)

func newSnapshotVerifyCommand() *cobra.Command {
	var (
		repositoryPath string
		snapshotID     string
	)

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify snapshot integrity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if repositoryPath == "" {
				return errors.New("--repo is required")
			}

			if snapshotID == "" {
				return errors.New("--id is required")
			}

			manifestRepo, err := disk.NewManifestRepository(
				repositoryPath,
			)
			if err != nil {
				return err
			}

			objectRepo, err := disk.NewObjectRepository(
				repositoryPath,
			)
			if err != nil {
				return err
			}

			verifier := snapshot.NewVerifier(
				objectRepo,
				manifestRepo,
			)

			result, err := verifier.Verify(
				cmd.Context(),
				snapshotID,
			)
			if err != nil {
				return err
			}

			if err := printVerifyResult(
				cmd.OutOrStdout(),
				result,
			); err != nil {
				return err
			}

			if !result.IsSuccess() {
				return fmt.Errorf(
					"snapshot %q failed verification",
					snapshotID,
				)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(
		&repositoryPath,
		"repo",
		"",
		"path to the Stowmark repository",
	)

	cmd.Flags().StringVar(
		&snapshotID,
		"id",
		"",
		"snapshot ID",
	)

	return cmd
}

func printVerifyResult(
	output io.Writer,
	result *snapshot.Result,
) error {
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
			"CHECKED:\t%d\n"+
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
