package cli

import (
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
	"github.com/spf13/cobra"
)

func newKeyRewrapCommand() *cobra.Command {
	var (
		repositoryPath string
		oldPrivateKey  string
		newPublicKey   string
	)
	cmd := &cobra.Command{
		Use:   "rewrap",
		Short: "Rotate encryption key pair",
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
			defer func() {
				_ = repoHandler.Close()
			}()
			evtr := app.NewEventsTracing()
			multiMdw, err := middlewares.BuildCommandMiddlewares(obsv, evtr)
			if err != nil {
				return err
			}
			locker, err := newRepositoryLocker(repoHandler)
			if err != nil {
				return fmt.Errorf("failed to create repository locker: %w", err)
			}
			handler := multiMdw(app.NewRewrapKey(repoHandler.FolderRepository(), asymmetricRepo, symmetricRepo, locker))
			_, err = handler.Handle(cmd.Context(), app.RewrapKeyCommand{
				RepositoryPath: repoHandler.RepositoryPath(),
				OldPrivateKey:  oldPrivateKey,
				NewPublicKey:   newPublicKey,
			})
			if err != nil {
				return fmt.Errorf("failed to rewrap: %w", err)
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
		&oldPrivateKey,
		"old-private-key",
		"",
		"old private key path",
	)
	cmd.Flags().StringVar(
		&newPublicKey,
		"new-public-key",
		"",
		"new public key path",
	)

	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("old-private-key")
	_ = cmd.MarkFlagRequired("new-public-key")
	return cmd
}
