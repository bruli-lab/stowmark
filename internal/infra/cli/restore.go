package cli

import (
	"fmt"

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

			repoHandler, err := repositories.NewHandler(cmd.Context(), repositoryPath)
			if err != nil {
				return err
			}
			defer func() {
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

			decryptKeySvc := encryption.NewDecryptSymmetricKey(encrypt.NewSymmetricRepository(), disk.NewAsymmetricKeyPairRepository())
			switch filePath {
			case "":
				return executeRestore(
					cmd,
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

func executeRestoreFile(
	cmd *cobra.Command,
	manifestRepo snapshot.ManifestRepository,
	objectRepo snapshot.ObjectRepository,
	folderRepository repository.FolderRepository,
	decryptKeySvc *encryption.DecryptSymmetricKey,
	snapshotID, filePath, repositoryPath string,
	destinationPath, privateKey *string,
) error {
	svc := snapshot.NewRestoreFile(manifestRepo, objectRepo, folderRepository, decryptKeySvc)
	if err := svc.Restore(cmd.Context(), snapshotID, filePath, repositoryPath, destinationPath, privateKey); err != nil {
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

func executeRestore(
	cmd *cobra.Command,
	manifestRepo snapshot.ManifestRepository,
	objectRepo snapshot.ObjectRepository,
	folderRepo repository.FolderRepository,
	decryptKeySvc *encryption.DecryptSymmetricKey,
	repositoryPath, snapshotID string,
	destination *string,
	privateKey *string,
) error {
	svc := snapshot.NewRestore(
		manifestRepo,
		objectRepo,
		folderRepo,
		decryptKeySvc,
	)

	result, err := svc.Restore(
		cmd.Context(),
		snapshotID,
		repositoryPath,
		destination,
		privateKey,
	)
	if err != nil {
		return err
	}

	if err := printResult(
		cmd.OutOrStdout(),
		result,
	); err != nil {
		return err
	}

	if !result.IsSuccess() {
		return fmt.Errorf(
			"snapshot %q failed restoration",
			snapshotID,
		)
	}

	return nil
}
