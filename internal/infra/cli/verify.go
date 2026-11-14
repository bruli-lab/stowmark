package cli

import (
	"errors"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/bootstrap"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
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
			return withRepository(cmd, repositoryPath, func(repo *bootstrap.Repository) error {
				result, err := app.QueryResult[*snapshot.Result](repo.VerifySnapshot().Handle(cmd.Context(), app.VerifySnapshotQuery{
					SnapshotID:     snapshotID,
					RepositoryPath: repo.Path(),
					PrivateKeyPath: optionalPath(privateKey),
				}))
				if err != nil {
					return err
				}
				if err := printResult(cmd.OutOrStdout(), result.SnapshotID(), result.TotalFiles(), failedResults(result.Failed()), result.IsSuccess()); err != nil {
					return err
				}
				if !result.IsSuccess() {
					return fmt.Errorf("snapshot %q failed verification", snapshotID)
				}
				return nil
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
		&privateKey,
		"private-key",
		"",
		"Private key for decryption",
	)
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}
