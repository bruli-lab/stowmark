package cli

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
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
			if dryRun {
				return runPrunePlan(cmd.Context(), cmd.OutOrStdout(), obsv, repoHandler)
			}
			return runPrune(cmd.Context(), cmd.OutOrStdout(), obsv, repoHandler)
		},
	}

	cmd.Flags().StringVar(&repositoryPath, "repo", "", "path to the Stowmark repository")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "only report the unreferenced objects, do not delete them")
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}

type pruneDependencies struct {
	folderRepo   app.FolderRepository
	manifestRepo app.ManifestRepository
	objectRepo   app.ObjectRepository
	locker       *app.RepositoryLocker
}

func newPruneDependencies(repoHandler repositories.Repositories) (*pruneDependencies, error) {
	manifestRepo, err := repoHandler.ManifestRepository()
	if err != nil {
		return nil, err
	}
	objectRepo, err := repoHandler.ObjectRepository()
	if err != nil {
		return nil, err
	}
	locker, err := newRepositoryLocker(repoHandler)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository locker: %w", err)
	}
	return &pruneDependencies{
		folderRepo:   repoHandler.FolderRepository(),
		manifestRepo: manifestRepo,
		objectRepo:   objectRepo,
		locker:       locker,
	}, nil
}

func runPrunePlan(ctx context.Context, output io.Writer, obsv *observabilityinfra.OTLPObservability, repoHandler repositories.Repositories) error {
	deps, err := newPruneDependencies(repoHandler)
	if err != nil {
		return err
	}
	queryMdw, err := middlewares.BuildQueryMiddlewares(obsv)
	if err != nil {
		return err
	}
	handler := queryMdw(app.NewPrunePlan(deps.folderRepo, deps.manifestRepo, deps.objectRepo, deps.locker))
	result, err := handler.Handle(ctx, app.PrunePlanQuery{RepositoryPath: repoHandler.RepositoryPath()})
	if err != nil {
		return fmt.Errorf("failed to plan prune: %w", err)
	}
	plan, ok := result.(*snapshot.PrunePlan)
	if !ok {
		return fmt.Errorf("unexpected result type: %T", result)
	}

	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintf(writer,
		"STORED OBJECTS:\t%d\nREFERENCED OBJECTS:\t%d\nWOULD REMOVE:\t%d\n",
		plan.StoredObjects(), plan.ReferencedObjects(), len(plan.Unreferenced()),
	); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return printMissingObjects(output, len(plan.Missing()))
}

func runPrune(ctx context.Context, output io.Writer, obsv *observabilityinfra.OTLPObservability, repoHandler repositories.Repositories) error {
	deps, err := newPruneDependencies(repoHandler)
	if err != nil {
		return err
	}
	multiMdw, err := middlewares.BuildCommandMiddlewares(obsv, app.NewEventsTracing())
	if err != nil {
		return err
	}
	handler := multiMdw(app.NewPruneRepository(deps.folderRepo, deps.manifestRepo, deps.objectRepo, deps.locker))
	events, err := handler.Handle(ctx, app.PruneRepositoryCommand{RepositoryPath: repoHandler.RepositoryPath()})
	if err != nil {
		return fmt.Errorf("failed to prune repository: %w", err)
	}

	removed := 0
	for _, pruned := range app.EventsOf[*snapshot.ObjectsPruned](events) {
		removed += pruned.Removed()
	}
	if _, err := fmt.Fprintf(output, "Removed %d unreferenced object(s)\n", removed); err != nil {
		return err
	}
	missing := 0
	for _, detected := range app.EventsOf[*snapshot.MissingObjectsDetected](events) {
		missing += len(detected.Hashes())
	}
	return printMissingObjects(output, missing)
}

func printMissingObjects(output io.Writer, missing int) error {
	if missing == 0 {
		return nil
	}
	_, err := fmt.Fprintf(output,
		"WARNING: %d referenced object(s) are missing from the repository; run snapshot verify to find the affected snapshots\n",
		missing,
	)
	return err
}
