package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/bootstrap"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
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
		Long: "Remove the stored objects that are not referenced by any snapshot manifest, the objects of old key " +
			"generations left by an interrupted key rekey and the temporary files left by interrupted writes. " +
			"It takes an exclusive repository lock, so no other command can use the repository while it runs.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withRepository(cmd, repositoryPath, func(repo *bootstrap.Repository) error {
				if dryRun {
					return runPrunePlan(cmd.Context(), cmd.OutOrStdout(), repo)
				}
				return runPrune(cmd.Context(), cmd.OutOrStdout(), repo)
			})
		},
	}

	cmd.Flags().StringVar(&repositoryPath, "repo", "", "path to the Stowmark repository")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "only report the unreferenced objects, do not delete them")
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}

func runPrunePlan(ctx context.Context, output io.Writer, repo *bootstrap.Repository) error {
	plan, err := app.QueryResult[*snapshot.PrunePlan](repo.PrunePlan().Handle(ctx, app.PrunePlanQuery{RepositoryPath: repo.Path()}))
	if err != nil {
		return fmt.Errorf("failed to plan prune: %w", err)
	}

	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintf(writer,
		"STORED OBJECTS:\t%d\nREFERENCED OBJECTS:\t%d\nWOULD REMOVE:\t%d\nOLD KEY GENERATIONS:\t%s\n",
		plan.StoredObjects(), plan.ReferencedObjects(), len(plan.Unreferenced()), formatGenerations(plan.OrphanGenerations()),
	); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return printMissingObjects(output, len(plan.Missing()))
}

func runPrune(ctx context.Context, output io.Writer, repo *bootstrap.Repository) error {
	events, err := repo.PruneRepository().Handle(ctx, app.PruneRepositoryCommand{RepositoryPath: repo.Path()})
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
	for _, generations := range app.EventsOf[*snapshot.GenerationsRemoved](events) {
		if _, err := fmt.Fprintf(output, "Removed old key generation(s): %s\n", formatGenerations(generations.Generations())); err != nil {
			return err
		}
	}
	for _, temporaries := range app.EventsOf[*snapshot.TemporariesRemoved](events) {
		if _, err := fmt.Fprintf(output, "Removed %d temporary file(s) left by interrupted writes\n", temporaries.Removed()); err != nil {
			return err
		}
	}
	missing := 0
	for _, detected := range app.EventsOf[*snapshot.MissingObjectsDetected](events) {
		missing += len(detected.Hashes())
	}
	return printMissingObjects(output, missing)
}

func formatGenerations(generations []uint64) string {
	if len(generations) == 0 {
		return "none"
	}
	names := make([]string, len(generations))
	for i, generation := range generations {
		names[i] = strconv.FormatUint(generation, 10)
	}
	return strings.Join(names, ", ")
}

func printMissingObjects(output io.Writer, missing int) error {
	if missing == 0 {
		return nil
	}
	_, err := fmt.Fprintf(output,
		"WARNING: %d referenced object(s) are missing from the repository; run check to find the affected snapshots\n",
		missing,
	)
	return err
}
