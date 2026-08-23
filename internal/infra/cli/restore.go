package cli

import (
	"fmt"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
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
			var destination *string

			if destinationPath != "" {
				destination = &destinationPath
			}
			var privateKey *string
			if privateKeyPath != "" {
				privateKey = &privateKeyPath
			}
			obsv, err := builtObservability(cmd.Context())
			if err != nil {
				return err
			}
			repoHandler, err := repositories.NewHandler(cmd.Context(), repositoryPath)
			if err != nil {
				return err
			}
			defer func() {
				_ = obsv.Shutdown(cmd.Context())
				_ = repoHandler.Close()
			}()

			manifestRepo, err := repoHandler.ManifestRepository()
			if err != nil {
				return err
			}

			objectRepo, err := repoHandler.ObjectRepository()
			if err != nil {
				return err
			}
			multiMdw, err := buildCommandMiddlewares(obsv, err)
			if err != nil {
				return err
			}
			decryptKeySvc := encryption.NewDecryptSymmetricKey(encrypt.NewSymmetricRepository(), disk.NewAsymmetricKeyPairRepository())
			switch filePath {
			case "":
				return executeRestore(
					cmd,
					multiMdw,
					manifestRepo,
					objectRepo,
					repoHandler.FolderRepository(),
					decryptKeySvc,
					repoHandler.RepositoryPath(),
					snapshotID,
					destination,
					privateKey,
				)
			default:
				return executeRestoreFile(
					cmd,
					multiMdw,
					manifestRepo,
					objectRepo,
					repoHandler.FolderRepository(),
					decryptKeySvc,
					snapshotID,
					filePath,
					repoHandler.RepositoryPath(),
					destination,
					privateKey,
				)
			}
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

func executeRestoreFile(cmd *cobra.Command, mdw cqs.CommandHandlerMiddleware, manifestRepo snapshot.ManifestRepository, objectRepo snapshot.ObjectRepository, folderRepository repository.FolderRepository, decryptKeySvc *encryption.DecryptSymmetricKey, snapshotID, filePath, repositoryPath string, destinationPath, privateKey *string) error {
	svc := snapshot.NewRestoreFile(manifestRepo, objectRepo, folderRepository, decryptKeySvc)
	handler := mdw(app.NewRestoreFile(svc))

	if _, err := handler.Handle(cmd.Context(), app.RestoreFileCommand{
		SnapshotID:      snapshotID,
		FilePath:        filePath,
		RepositoryPath:  repositoryPath,
		DestinationPath: destinationPath,
		PrivateKeyPath:  privateKey,
	}); err != nil {
		return err
	}
	_, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"Restored file %q from snapshot %q\n",
		filePath,
		snapshotID,
	)
	return err
}

func executeRestore(cmd *cobra.Command, mdw cqs.CommandHandlerMiddleware, manifestRepo snapshot.ManifestRepository, objectRepo snapshot.ObjectRepository, folderRepo repository.FolderRepository, decryptKeySvc *encryption.DecryptSymmetricKey, repositoryPath, snapshotID string, destination, privateKey *string) error {
	svc := snapshot.NewRestore(
		manifestRepo,
		objectRepo,
		folderRepo,
		decryptKeySvc,
	)
	hadler := mdw(app.NewRestoreSnapshot(svc))
	events, err := hadler.Handle(cmd.Context(), app.RestoreSnapshotCommand{
		SnapshotID:      snapshotID,
		RepositoryPath:  repositoryPath,
		PrivateKeyPath:  privateKey,
		DestinationPath: destination,
	})
	if err != nil {
		return err
	}

	if len(events) != 1 {
		return fmt.Errorf("unexpected number of events")
	}
	result, ok := events[0].(*app.RestoreSnapshotEvent)
	if !ok {
		return fmt.Errorf("unexpected event type")
	}
	failed := make([]Failed, len(result.FailedFiles))
	for i, f := range result.FailedFiles {
		failed[i] = Failed{
			Path:   f.Path,
			Reason: f.Reason,
		}
	}
	if err := printResult(cmd.OutOrStdout(), result.SnapshotID, result.TotalFiles, failed, result.IsSuccess); err != nil {
		return err
	}

	if !result.IsSuccess {
		return fmt.Errorf(
			"snapshot %q failed restoration",
			snapshotID,
		)
	}

	return nil
}
