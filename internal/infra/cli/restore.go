package cli

import (
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/bootstrap"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/spf13/cobra"
)

func newSnapshotRestoreCommand() *cobra.Command {
	var (
		repositoryPath  string
		snapshotID      string
		filePath        string
		destinationPath string
		privateKeyPath  string
	)

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore snapshot",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			destination := optionalPath(destinationPath)
			privateKey := optionalPath(privateKeyPath)
			return withRepository(cmd, repositoryPath, func(repo *bootstrap.Repository) error {
				if filePath != "" {
					return executeRestoreFile(cmd, repo, snapshotID, filePath, destination, privateKey)
				}
				return executeRestore(cmd, repo, snapshotID, destination, privateKey)
			})
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

	cmd.Flags().StringVar(
		&filePath,
		"file",
		"",
		"path to the file to restore",
	)
	cmd.Flags().StringVar(
		&destinationPath,
		"destination",
		"",
		"destination path",
	)
	cmd.Flags().StringVar(
		&privateKeyPath,
		"private-key",
		"",
		"private key for decryption",
	)

	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}

func executeRestoreFile(cmd *cobra.Command, repo *bootstrap.Repository, snapshotID, filePath string, destination, privateKey *string) error {
	if _, err := repo.RestoreFile().Handle(cmd.Context(), app.RestoreFileCommand{
		SnapshotID:      snapshotID,
		FilePath:        filePath,
		RepositoryPath:  repo.Path(),
		DestinationPath: destination,
		PrivateKeyPath:  privateKey,
	}); err != nil {
		return err
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Restored file %q from snapshot %q\n", filePath, snapshotID)
	return err
}

func executeRestore(cmd *cobra.Command, repo *bootstrap.Repository, snapshotID string, destination, privateKey *string) error {
	events, err := repo.RestoreSnapshot().Handle(cmd.Context(), app.RestoreSnapshotCommand{
		SnapshotID:      snapshotID,
		RepositoryPath:  repo.Path(),
		PrivateKeyPath:  privateKey,
		DestinationPath: destination,
	})
	if err != nil {
		return err
	}
	result, err := app.SingleEvent[*snapshot.SnapshotRestored](events)
	if err != nil {
		return err
	}
	if err := printResult(cmd.OutOrStdout(), result.SnapshotID(), result.TotalFiles(), failedResults(result.Failed()), result.IsSuccess()); err != nil {
		return err
	}
	if !result.IsSuccess() {
		return fmt.Errorf("snapshot %q failed restoration", snapshotID)
	}
	return nil
}
