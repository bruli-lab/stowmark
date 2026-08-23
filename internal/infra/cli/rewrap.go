package cli

import (
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
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
			folderRepo := disk.NewFolderRepositoryRepository()
			svc := repository.NewRewrap(folderRepo, symmetricRepo, asymmetricRepo)
			multiMdw, err := buildCommandMiddlewares(obsv, err)
			if err != nil {
				return err
			}
			handler := multiMdw(app.NewRewrapKey(svc))
			_, err = handler.Handle(cmd.Context(), app.RewrapKeyCommand{
				RepositoryPath: repositoryPath,
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
