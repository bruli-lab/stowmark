package cli

import (
	"errors"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
	"github.com/spf13/cobra"
)

func newSnapshotVerifyCommand() *cobra.Command {
	var (
		repositoryPath string
		snapshotID     string
		privateKey     string
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

			objectRepo, err := repoHand.ObjectRepository()
			if err != nil {
				return err
			}

			verifier := snapshot.NewVerifier(
				objectRepo,
				manifestRepo,
				repoHand.FolderRepository(),
				encryption.NewDecryptSymmetricKey(encrypt.NewSymmetricRepository(), disk.NewAsymmetricKeyPairRepository()),
			)
			var privateKeyPath *string
			if privateKey != "" {
				privateKeyPath = &privateKey
			}

			result, err := verifier.Verify(
				cmd.Context(),
				repoHand.RepositoryPath(),
				snapshotID,
				privateKeyPath,
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
				return fmt.Errorf("snapshot %q failed verification", snapshotID)
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
	cmd.Flags().StringVar(
		&privateKey,
		"private-key",
		"",
		"Private key for decryption",
	)
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}
