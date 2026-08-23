package cli

import (
	"errors"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
	"github.com/spf13/cobra"
)

func newSnapshotCreateCommand() *cobra.Command {
	var (
		repositoryPath string
		privateKeyPath string
	)
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

			repoHand, err := repositories.NewHandler(cmd.Context(), repositoryPath)
			if err != nil {
				return err
			}
			defer func() {
				_ = repoHand.Close()
			}()

			manifestRepo, err := repoHand.ManifestRepository()
			if err != nil {
				return err
			}
			objRepo, err := repoHand.ObjectRepository()
			if err != nil {
				return err
			}
			folderRepositoryRepo := repoHand.FolderRepository()
			decryptSymmetricKeySvc := encryption.NewDecryptSymmetricKey(encrypt.NewSymmetricRepository(), disk.NewAsymmetricKeyPairRepository())
			obsv, err := builtObservability(cmd.Context())
			if err != nil {
				return err
			}
			defer func() {
				_ = obsv.Shutdown(cmd.Context())
			}()
			multiMdw, err := buildCommandMiddlewares(obsv, err)
			if err != nil {
				return err
			}
			svc := snapshot.NewCreate(sourceRepo, manifestRepo, objRepo, repository.NewGetConfig(folderRepositoryRepo), decryptSymmetricKeySvc)
			handler := multiMdw(app.NewCreateSnapshot(svc))

			var privateKey *string
			if privateKeyPath != "" {
				privateKey = &privateKeyPath
			}

			events, err := handler.Handle(cmd.Context(), app.CreateSnapshotCommand{
				RepositoryPath: repoHand.RepositoryPath(),
				SourcePath:     sourcePath,
				PrivateKey:     privateKey,
			})
			if err != nil {
				return err
			}

			if len(events) != 1 {
				return errors.New("unexpected number of events")
			}
			result, ok := events[0].(*app.CreateSnapshotEvent)
			if !ok {
				return errors.New("unexpected event type")
			}

			_, _ = fmt.Fprintf(
				cmd.OutOrStdout(),
				"Snapshot created: %s\nFiles: %d\nSize: %d bytes\n",
				result.SnapshotID,
				result.FileCount,
				result.TotalSize,
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

	cmd.Flags().StringVar(
		&privateKeyPath,
		"private-key",
		"",
		"path to the private key for encryption",
	)
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}
