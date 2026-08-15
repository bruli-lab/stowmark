package cli

import (
	"errors"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
	"github.com/spf13/cobra"
)

func newSnapshotVerifyCommand() *cobra.Command {
	var (
		repositoryPath string
		snapshotID     string
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
			)

			result, err := verifier.Verify(
				cmd.Context(),
				snapshotID,
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

	return cmd
}
