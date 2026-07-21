package cli

import (
	"errors"
	"fmt"

	"github.com/bruli-lab/stonekeep.git/internal/domain/snapshot"
	"github.com/bruli-lab/stonekeep.git/internal/infra/disk"
	"github.com/spf13/cobra"
)

func newSnapshotCommand() *cobra.Command {
	var repositoryPath string
	sourceRepo := disk.NewSourceRepository()

	cmd := &cobra.Command{
		Use:   "snapshot <source>",
		Short: "Create a snapshot of a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourcePath := args[0]

			if repositoryPath == "" {
				return errors.New("--repo is required")
			}

			manifestRepo := disk.NewManifestRepository(repositoryPath)
			objRepo := disk.NewObjectRepository(repositoryPath)
			create := snapshot.NewCreate(sourceRepo, manifestRepo, objRepo)

			result, err := create.Do(
				cmd.Context(),
				sourcePath,
			)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(
				cmd.OutOrStdout(),
				"Snapshot created: %s\nFiles: %d\nSize: %d bytes\n",
				result.Id(),
				result.FileCount(),
				result.TotalSize(),
			)

			return nil
		},
	}

	cmd.Flags().StringVar(
		&repositoryPath,
		"repo",
		"",
		"path to the Stonekeep repository",
	)

	return cmd
}
