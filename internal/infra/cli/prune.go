package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/bruli-lab/stowmark/internal/infra/progress"

	"github.com/bruli-lab/go-core/event"
	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/bootstrap"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/spf13/cobra"
)

func newPruneCommand() *cobra.Command {
	var (
		repositoryPath string
		privateKeyPath string
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
			privateKey := optionalPath(privateKeyPath)
			return withRepository(cmd, repositoryPath, func(repo *bootstrap.Repository) error {
				if dryRun {
					return runPrunePlan(cmd, repo, privateKey)
				}
				result, err := runPrune(cmd.Context(), repo, privateKey)
				if err != nil {
					return err
				}
				return render(cmd, result, result.print)
			})
		},
	}

	cmd.Flags().StringVar(&repositoryPath, "repo", "", "path to the Stowmark repository")
	cmd.Flags().StringVar(&privateKeyPath, "private-key", "", metadataPrivateKeyUsage)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "only report the unreferenced objects, do not delete them")
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}

func runPrunePlan(cmd *cobra.Command, repo *bootstrap.Repository, privateKey *string) error {
	plan, err := app.QueryResult[*snapshot.PrunePlan](repo.PrunePlan().Handle(cmd.Context(), app.PrunePlanQuery{
		RepositoryPath: repo.Path(),
		PrivateKeyPath: privateKey,
	}))
	if err != nil {
		return fmt.Errorf("failed to plan prune: %w", err)
	}
	return render(cmd, newPrunePlanDocument(plan), func(output io.Writer) error {
		return printPrunePlan(output, plan)
	})
}

func printPrunePlan(output io.Writer, plan *snapshot.PrunePlan) error {
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintf(writer,
		"STORED OBJECTS:\t%d\nREFERENCED OBJECTS:\t%d\nWOULD REMOVE:\t%d (%s)\nOLD KEY GENERATIONS:\t%s\n",
		plan.StoredObjects(), plan.ReferencedObjects(), len(plan.Unreferenced()), progress.FormatBytes(plan.UnreferencedBytes()),
		formatGenerations(plan.OrphanGenerations()),
	); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return printMissingObjects(output, len(plan.Missing()))
}

func runPrune(ctx context.Context, repo *bootstrap.Repository, privateKey *string) (*pruneDocument, error) {
	events, err := repo.PruneRepository().Handle(ctx, app.PruneRepositoryCommand{
		RepositoryPath: repo.Path(),
		PrivateKeyPath: privateKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to prune repository: %w", err)
	}
	return newPruneDocument(events), nil
}

func newPruneDocument(events []event.Event) *pruneDocument {
	result := &pruneDocument{RemovedKeyGenerations: []uint64{}, PostponedKeyGenerations: []uint64{}}
	for _, pruned := range app.EventsOf[*snapshot.ObjectsPruned](events) {
		result.RemovedObjects += pruned.Removed()
		result.FreedBytes += pruned.FreedBytes()
	}
	for _, generations := range app.EventsOf[*snapshot.GenerationsRemoved](events) {
		result.RemovedKeyGenerations = append(result.RemovedKeyGenerations, generations.Generations()...)
	}
	for _, temporaries := range app.EventsOf[*snapshot.TemporariesRemoved](events) {
		result.RemovedTemporaryFiles += temporaries.Removed()
	}
	for _, detected := range app.EventsOf[*snapshot.MissingObjectsDetected](events) {
		result.MissingObjects += len(detected.Hashes())
	}
	for _, postponed := range app.EventsOf[*snapshot.ObjectsDeletionPostponed](events) {
		until := postponed.Until()
		result.PostponedObjects += postponed.Objects()
		result.PostponedBytes += postponed.Bytes()
		result.PostponedKeyGenerations = append(result.PostponedKeyGenerations, postponed.Generations()...)
		result.RetainedUntil = &until
	}
	return result
}

func (p *pruneDocument) print(output io.Writer) error {
	if _, err := fmt.Fprintf(output, "Removed %d unreferenced object(s), freed %s\n", p.RemovedObjects, progress.FormatBytes(p.FreedBytes)); err != nil {
		return err
	}
	if len(p.RemovedKeyGenerations) > 0 {
		if _, err := fmt.Fprintf(output, "Removed old key generation(s): %s\n", formatGenerations(p.RemovedKeyGenerations)); err != nil {
			return err
		}
	}
	if p.RetainedUntil != nil {
		if _, err := fmt.Fprintf(output, "Kept %d object(s) (%s) and old key generation(s) %s under object lock retention until %s; run prune again after that\n",
			p.PostponedObjects, progress.FormatBytes(p.PostponedBytes), formatGenerations(p.PostponedKeyGenerations), formatLocal(*p.RetainedUntil),
		); err != nil {
			return err
		}
	}
	if p.RemovedTemporaryFiles > 0 {
		if _, err := fmt.Fprintf(output, "Removed %d temporary file(s) left by interrupted writes\n", p.RemovedTemporaryFiles); err != nil {
			return err
		}
	}
	return printMissingObjects(output, p.MissingObjects)
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
