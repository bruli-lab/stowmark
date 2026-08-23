package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
	"github.com/spf13/cobra"
)

func newSnapshotListCommand() *cobra.Command {
	var repositoryPath string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List repository snapshots",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			obsv, err := builtObservability(cmd.Context())
			if err != nil {
				return fmt.Errorf("create observability: %w", err)
			}
			repoHand, err := repositories.NewHandler(cmd.Context(), repositoryPath)
			if err != nil {
				return err
			}
			defer func() {
				_ = obsv.Shutdown(cmd.Context())
				_ = repoHand.Close()
			}()

			manifestRepo, err := repoHand.ManifestRepository()
			if err != nil {
				return err
			}

			svc := snapshot.NewListing(manifestRepo)
			multiMdw, err := middlewares.BuildQueryMiddlewares(obsv)
			if err != nil {
				return err
			}
			handler := multiMdw(app.NewSnapshotList(svc))

			result, err := handler.Handle(cmd.Context(), app.SnaphotListQuery{})
			snapshots, ok := result.([]snapshot.ManifestResume)
			if !ok {
				panic("unexpected result type")
			}
			if err != nil {
				return err
			}

			if len(snapshots) == 0 {
				_, err = fmt.Fprintln(
					cmd.OutOrStdout(),
					"No snapshots found.",
				)

				return err
			}

			writer := tabwriter.NewWriter(
				cmd.OutOrStdout(),
				0, // amplada mínima
				4, // amplada del tabulador
				2, // espais entre columnes
				' ',
				0,
			)

			_, err = fmt.Fprintln(
				writer,
				"ID\tCREATED AT\tFILES\tSIZE\tSOURCE",
			)
			if err != nil {
				return err
			}

			for _, item := range snapshots {
				created := item.CreatedAt().In(time.Local)
				_, err = fmt.Fprintf(
					writer,
					"%s\t%s\t%d\t%s\t%s\n",
					item.Id(),
					created.Format("2006-01-02 15:04:05"),
					item.Files(),
					formatBytes(item.Size()),
					item.Source(),
				)
				if err != nil {
					return err
				}
			}

			return writer.Flush()
		},
	}

	cmd.Flags().StringVar(
		&repositoryPath,
		"repo",
		"",
		"path to the Stowmark repository",
	)

	_ = cmd.MarkFlagRequired("repo")

	return cmd
}

func formatBytes(size int64) string {
	const unit = 1024

	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	divisor := int64(unit)
	exponent := 0

	for value := size / unit; value >= unit; value /= unit {
		divisor *= unit
		exponent++
	}

	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(divisor), "KMGTPE"[exponent])
}
