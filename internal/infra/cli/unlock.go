package cli

import (
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/bootstrap"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/spf13/cobra"
)

func newUnlockCommand() *cobra.Command {
	var (
		repositoryPath string
		all            bool
	)

	cmd := &cobra.Command{
		Use:   "unlock",
		Short: "Remove stale repository locks",
		Long: "Remove repository locks that have not been refreshed for " +
			"30 minutes. With --all, remove every lock, including locks held by running commands.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withRepository(cmd, repositoryPath, func(repo *bootstrap.Repository) error {
				events, err := repo.Unlock().Handle(cmd.Context(), app.UnlockCommand{
					RepositoryPath: repo.Path(),
					All:            all,
				})
				if err != nil {
					return fmt.Errorf("failed to unlock repository: %w", err)
				}
				removed := len(app.EventsOf[*repository.LockRemoved](events))
				active, err := app.QueryResult[[]repository.Lock](repo.LockList().Handle(cmd.Context(), app.LockListQuery{}))
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Removed %d lock(s), %d active lock(s) left\n", removed, len(active))
				return err
			})
		},
	}

	cmd.Flags().StringVar(
		&repositoryPath,
		"repo",
		"",
		"path to the Stowmark repository",
	)
	cmd.Flags().BoolVar(
		&all,
		"all",
		false,
		"remove every lock, including locks held by running commands",
	)
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}
