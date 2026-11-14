package cli

import (
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
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
			obsv, err := buildObservability(cmd.Context())
			if err != nil {
				return fmt.Errorf("create observability: %w", err)
			}
			repoHandler, err := repositories.NewHandler(cmd.Context(), repositoryPath)
			if err != nil {
				return err
			}
			defer func() {
				_ = obsv.Shutdown(cmd.Context())
				_ = repoHandler.Close()
			}()
			lockRepo, err := repoHandler.LockRepository()
			if err != nil {
				return err
			}
			multiMdw, err := middlewares.BuildCommandMiddlewares(obsv, app.NewEventsTracing())
			if err != nil {
				return err
			}
			handler := multiMdw(app.NewUnlock(lockRepo))
			events, err := handler.Handle(cmd.Context(), app.UnlockCommand{
				RepositoryPath: repoHandler.RepositoryPath(),
				All:            all,
			})
			if err != nil {
				return fmt.Errorf("failed to unlock repository: %w", err)
			}
			removed := len(app.EventsOf[*repository.LockRemoved](events))
			queryMdw, err := middlewares.BuildQueryMiddlewares(obsv)
			if err != nil {
				return err
			}
			result, err := queryMdw(app.NewLockList(lockRepo)).Handle(cmd.Context(), app.LockListQuery{})
			if err != nil {
				return err
			}
			active, ok := result.([]repository.Lock)
			if !ok {
				return fmt.Errorf("unexpected result type: %T", result)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Removed %d lock(s), %d active lock(s) left\n", removed, len(active))
			return err
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
