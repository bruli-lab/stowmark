package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
	observabilityinfra "github.com/bruli-lab/stowmark/internal/infra/observability"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
	"github.com/spf13/cobra"
)

func newPruneCommand() *cobra.Command {
	var (
		repositoryPath string
		dryRun         bool
	)

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove objects that no snapshot references",
		Long: "Remove the stored objects that are not referenced by any snapshot manifest. " +
			"It takes an exclusive repository lock, so no other command can use the repository while it runs.",
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
			return runPrune(cmd.Context(), cmd.OutOrStdout(), obsv, repoHandler, dryRun)
		},
	}

	cmd.Flags().StringVar(&repositoryPath, "repo", "", "path to the Stowmark repository")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "only report the unreferenced objects, do not delete them")
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}

func runPrune(ctx context.Context, output io.Writer, obsv *observabilityinfra.OTLPObservability, repoHandler repositories.Repositories, dryRun bool) error {
	manifestRepo, err := repoHandler.ManifestRepository()
	if err != nil {
		return err
	}
	objectRepo, err := repoHandler.ObjectRepository()
	if err != nil {
		return err
	}
	locker, err := newRepositoryLocker(repoHandler)
	if err != nil {
		return fmt.Errorf("failed to create repository locker: %w", err)
	}
	multiMdw, err := middlewares.BuildCommandMiddlewares(obsv, app.NewEventsTracing())
	if err != nil {
		return err
	}
	handler := multiMdw(app.NewPruneRepository(repoHandler.FolderRepository(), manifestRepo, objectRepo, locker))
	events, err := handler.Handle(ctx, app.PruneRepositoryCommand{
		RepositoryPath: repoHandler.RepositoryPath(),
		DryRun:         dryRun,
	})
	if err != nil {
		return fmt.Errorf("failed to prune repository: %w", err)
	}
	if len(events) != 1 {
		return errors.New("unexpected number of events")
	}
	result, ok := events[0].(*app.PruneRepositoryEvent)
	if !ok {
		return fmt.Errorf("unexpected event type: %T", events[0])
	}
	return printPruneResult(output, result)
}

func printPruneResult(output io.Writer, result *app.PruneRepositoryEvent) error {
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	removedLabel := "REMOVED:"
	if result.DryRun {
		removedLabel = "WOULD REMOVE:"
	}
	if _, err := fmt.Fprintf(writer,
		"STORED OBJECTS:\t%d\nREFERENCED OBJECTS:\t%d\n%s\t%d\n",
		result.StoredObjects, result.ReferencedObjects, removedLabel, result.UnreferencedObjects,
	); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if result.MissingObjects == 0 {
		return nil
	}
	_, err := fmt.Fprintf(output,
		"WARNING: %d referenced object(s) are missing from the repository; run snapshot verify to find the affected snapshots\n",
		result.MissingObjects,
	)
	return err
}
