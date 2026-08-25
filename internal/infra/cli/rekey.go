package cli

import (
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
	"github.com/spf13/cobra"
)

func newKeyReKeyCommand() *cobra.Command {
	var (
		repositoryPath string
		privateKey     string
		publicKey      string
	)
	cmd := &cobra.Command{
		Use:   "rekey",
		Short: "Rotate symmetric key and rewrap all objects",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			obsv, err := builtObservability(cmd.Context())
			if err != nil {
				return fmt.Errorf("create observability: %w", err)
			}
			defer func() {
				_ = obsv.Shutdown(cmd.Context())
			}()
			asymmetricRepo := disk.NewAsymmetricKeyPairRepository()
			symmetricRepo := encrypt.NewSymmetricRepository()
			repoHandler, err := repositories.NewHandler(cmd.Context(), repositoryPath)
			if err != nil {
				return fmt.Errorf("failed to create repository handler: %w", err)
			}
			folderRepo := repoHandler.FolderRepository()
			objectRepo, err := repoHandler.ObjectRepository()
			if err != nil {
				return fmt.Errorf("failed to create object repository: %w", err)
			}
			svc := snapshot.NewReKey(folderRepo, symmetricRepo, asymmetricRepo, objectRepo)
			evtr := app.NewEventsTracing()
			multiMdw, err := middlewares.BuildCommandMiddlewares(obsv, evtr)
			if err != nil {
				return err
			}
			handler := multiMdw(app.NewRekeyKey(svc))
			_, err = handler.Handle(cmd.Context(), app.RekeyKeyCommand{
				RepositoryPath: repoHandler.RepositoryPath(),
				PrivateKeyPath: privateKey,
				PublicKeyPath:  publicKey,
			})
			if err != nil {
				return fmt.Errorf("failed to rekey: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "rotate encryption key pair successful")
			return err
		},
	}

	cmd.Flags().StringVar(
		&repositoryPath,
		"repo",
		"",
		"Repository path",
	)
	cmd.Flags().StringVar(
		&privateKey,
		"private-key",
		"",
		"private key path",
	)
	cmd.Flags().StringVar(
		&publicKey,
		"public-key",
		"",
		"public key path",
	)

	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("private-key")
	_ = cmd.MarkFlagRequired("public-key")
	return cmd
}
