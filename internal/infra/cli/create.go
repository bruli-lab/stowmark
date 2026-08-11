package cli

import (
	"errors"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/spf13/cobra"
)

func newSnapshotCreateCommand() *cobra.Command {
	var repositoryPath string
	sourceRepo := disk.NewSourceRepository()

	cmd := &cobra.Command{
		Use:   "create <source>",
		Short: "Create a snapshot of a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourcePath := args[0]

			if repositoryPath == "" {
				return errors.New("--repo is required")
			}

			repoHand, err := NewRepositoriesHandler(repositoryPath)
			if err != nil {
				return err
			}

			manifestRepo, err := repoHand.ManifestRepository()
			if err != nil {
				return err
			}
			objRepo, err := repoHand.ObjectRepository()
			if err != nil {
				return err
			}
			folderRepositoryRepo := repoHand.FolderRepository()
			create := snapshot.NewCreate(sourceRepo, manifestRepo, objRepo, repository.NewGetConfig(folderRepositoryRepo))

			result, err := create.Do(
				cmd.Context(),
				repoHand.RepositoryPath(),
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
		"path to the Stowmark repository",
	)

	return cmd
}
