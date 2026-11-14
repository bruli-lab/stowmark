package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
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
			manifestRepo, err := repoHandler.ManifestRepository()
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
			events, err := multiMdw(app.NewForgetSnapshots(manifestRepo, locker)).Handle(cmd.Context(), app.ForgetSnapshotsCommand{
				RepositoryPath: repoHandler.RepositoryPath(),
				Policy:         policy,
				DryRun:         dryRun,
			})
			if err != nil {
				return fmt.Errorf("failed to forget snapshots: %w", err)
			}
			if len(events) != 1 {
				return errors.New("unexpected number of events")
			}
			result, ok := events[0].(*app.ForgetSnapshotsEvent)
			if !ok {
				return fmt.Errorf("unexpected event type: %T", events[0])
			}
			if err := printForgetResult(cmd.OutOrStdout(), result); err != nil {
				return err
			}
			if !prune {
				return nil
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
				return err
			}
			return runPrune(cmd.Context(), cmd.OutOrStdout(), obsv, repoHandler, false)
		},
	}

	cmd.Flags().StringVar(&repositoryPath, "repo", "", "path to the Stowmark repository")
	cmd.Flags().IntVar(&rules.Last, "keep-last", 0, "keep the N most recent snapshots")
	cmd.Flags().IntVar(&rules.Daily, "keep-daily", 0, "keep the most recent snapshot of the last N days that have snapshots")
	cmd.Flags().IntVar(&rules.Weekly, "keep-weekly", 0, "keep the most recent snapshot of the last N weeks that have snapshots")
	cmd.Flags().IntVar(&rules.Monthly, "keep-monthly", 0, "keep the most recent snapshot of the last N months that have snapshots")
	cmd.Flags().IntVar(&rules.Yearly, "keep-yearly", 0, "keep the most recent snapshot of the last N years that have snapshots")
	cmd.Flags().StringVar(&within, "keep-within", "", "keep every snapshot newer than this duration (for example 30d, 2w or 36h)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "only show which snapshots would be forgotten")
	cmd.Flags().BoolVar(&prune, "prune", false, "run prune after forgetting the snapshots")
	cmd.MarkFlagsMutuallyExclusive("dry-run", "prune")
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}

func printForgetResult(output io.Writer, result *app.ForgetSnapshotsEvent) error {
	if len(result.Decisions) == 0 {
		_, err := fmt.Fprintln(output, "No snapshots found.")
		return err
	}

	forgetAction := "forget"
	if result.DryRun {
		forgetAction = "would forget"
	}

	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "ID\tCREATED AT\tSOURCE\tACTION\tREASONS"); err != nil {
		return err
	}
	for _, decision := range result.Decisions {
		item := decision.Snapshot()
		action := forgetAction
		if decision.Keep() {
			action = "keep"
		}
		reasons := make([]string, len(decision.Reasons()))
		for i, reason := range decision.Reasons() {
			reasons[i] = string(reason)
		}
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
			item.ID(),
			item.CreatedAt().In(time.Local).Format(time.DateTime),
			item.Source(),
			action,
			strings.Join(reasons, ", "),
		); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}

	forgotten := result.Forgotten()
	_, err := fmt.Fprintf(output, "\nKept %d snapshot(s), %s %d snapshot(s)\n", len(result.Decisions)-forgotten, pastForgetAction(result.DryRun), forgotten)
	return err
}

func pastForgetAction(dryRun bool) string {
	if dryRun {
		return "would forget"
	}
	return "forgot"
}
