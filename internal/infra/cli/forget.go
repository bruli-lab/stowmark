package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/bootstrap"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/spf13/cobra"
)

func newForgetCommand() *cobra.Command {
	var (
		repositoryPath string
		rules          snapshot.RetentionRules
		within         string
		dryRun         bool
		prune          bool
	)

	cmd := &cobra.Command{
		Use:   "forget",
		Short: "Remove the snapshots that a retention policy does not keep",
		Long: "Apply a retention policy to the snapshots of each source and delete the manifests it does not keep. " +
			"Forgetting snapshots does not free space; run prune afterwards, or use --prune.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			duration, err := parseRetentionDuration(within)
			if err != nil {
				return err
			}
			rules.Within = duration
			policy, err := snapshot.NewRetentionPolicy(rules)
			if err != nil {
				return err
			}
			return withRepository(cmd, repositoryPath, func(repo *bootstrap.Repository) error {
				if dryRun {
					return runForgetPlan(cmd.Context(), cmd.OutOrStdout(), repo, policy)
				}
				events, err := repo.ForgetSnapshots().Handle(cmd.Context(), app.ForgetSnapshotsCommand{
					RepositoryPath: repo.Path(),
					Policy:         policy,
				})
				if err != nil {
					return fmt.Errorf("failed to forget snapshots: %w", err)
				}
				if err := printForgotten(cmd.OutOrStdout(), app.EventsOf[*snapshot.SnapshotForgotten](events)); err != nil {
					return err
				}
				if !prune {
					return nil
				}
				if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
					return err
				}
				return runPrune(cmd.Context(), cmd.OutOrStdout(), repo)
			})
		},
	}

	cmd.Flags().StringVar(&repositoryPath, "repo", "", "path to the Stowmark repository")
	cmd.Flags().IntVar(&rules.Last, "keep-last", 0, "keep the N most recent snapshots")
	cmd.Flags().IntVar(&rules.Daily, "keep-daily", 0, "keep the most recent snapshot of the last N days that have snapshots")
	cmd.Flags().IntVar(&rules.Weekly, "keep-weekly", 0, "keep the most recent snapshot of the last N weeks that have snapshots")
	cmd.Flags().IntVar(&rules.Monthly, "keep-monthly", 0, "keep the most recent snapshot of the last N months that have snapshots")
	cmd.Flags().IntVar(&rules.Yearly, "keep-yearly", 0, "keep the most recent snapshot of the last N years that have snapshots")
	cmd.Flags().StringVar(&within, "keep-within", "", "keep every snapshot newer than this duration (for example 30d, 2w or 36h)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "only show which snapshots would be kept or forgotten, and why")
	cmd.Flags().BoolVar(&prune, "prune", false, "run prune after forgetting the snapshots")
	cmd.MarkFlagsMutuallyExclusive("dry-run", "prune")
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}

func runForgetPlan(ctx context.Context, output io.Writer, repo *bootstrap.Repository, policy *snapshot.RetentionPolicy) error {
	decisions, err := app.QueryResult[[]snapshot.RetentionDecision](repo.ForgetPlan().Handle(ctx, app.ForgetPlanQuery{Policy: policy}))
	if err != nil {
		return fmt.Errorf("failed to plan forget: %w", err)
	}
	if len(decisions) == 0 {
		_, err := fmt.Fprintln(output, "No snapshots found.")
		return err
	}

	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "ID\tCREATED AT\tSOURCE\tACTION\tREASONS"); err != nil {
		return err
	}
	forgotten := 0
	for i := range decisions {
		item := decisions[i].Snapshot()
		action := "keep"
		if !decisions[i].Keep() {
			action = "would forget"
			forgotten++
		}
		reasons := make([]string, len(decisions[i].Reasons()))
		for j, reason := range decisions[i].Reasons() {
			reasons[j] = string(reason)
		}
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
			item.ID(), formatLocal(item.CreatedAt()), item.Source(), action, strings.Join(reasons, ", "),
		); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "\nWould keep %d snapshot(s) and forget %d snapshot(s)\n", len(decisions)-forgotten, forgotten)
	return err
}

func printForgotten(output io.Writer, forgotten []*snapshot.SnapshotForgotten) error {
	if len(forgotten) > 0 {
		writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
		if _, err := fmt.Fprintln(writer, "FORGOTTEN\tCREATED AT\tSOURCE"); err != nil {
			return err
		}
		for _, ev := range forgotten {
			if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\n", ev.SnapshotID(), formatLocal(ev.CreatedAt()), ev.Source()); err != nil {
				return err
			}
		}
		if err := writer.Flush(); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(output, "Forgot %d snapshot(s)\n", len(forgotten))
	return err
}

func formatLocal(at time.Time) string {
	return at.In(time.Local).Format(time.DateTime)
}
