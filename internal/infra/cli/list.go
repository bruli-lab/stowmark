package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/bootstrap"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/spf13/cobra"
)

func newSnapshotListCommand() *cobra.Command {
	var repositoryPath string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List repository snapshots",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withRepository(cmd, repositoryPath, func(repo *bootstrap.Repository) error {
				snapshots, err := app.QueryResult[[]snapshot.ManifestResume](repo.SnapshotList().Handle(cmd.Context(), app.SnapshotListQuery{}))
				if err != nil {
					return err
				}
				if len(snapshots) == 0 {
					_, err = fmt.Fprintln(cmd.OutOrStdout(), "No snapshots found.")
					return err
				}
				writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
				if _, err := fmt.Fprintln(writer, "ID\tCREATED AT\tFILES\tSIZE\tSOURCE"); err != nil {
					return err
				}
				for _, item := range snapshots {
					if _, err := fmt.Fprintf(writer, "%s\t%s\t%d\t%s\t%s\n",
						item.ID(), formatLocal(item.CreatedAt()), item.Files(), formatBytes(item.Size()), item.Source(),
					); err != nil {
						return err
					}
				}
				return writer.Flush()
			})
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
