package cli

import (
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/bruli-lab/stowmark.git/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark.git/internal/infra/disk"
	"github.com/spf13/cobra"
)

func newSnapshotListCommand() *cobra.Command {
	var repositoryPath string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List repository snapshots",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if repositoryPath == "" {
				return errors.New("--repo is required")
			}

			manifestRepo := disk.NewManifestRepository(repositoryPath)

			list := snapshot.NewListing(manifestRepo)

			snapshots, err := list.List(cmd.Context())
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
				_, err = fmt.Fprintf(
					writer,
					"%s\t%s\t%d\t%s\t%s\n",
					item.Id(),
					item.CreatedAt().Format("2006-01-02 15:04:05"),
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

	return fmt.Sprintf(
		"%.1f %ciB",
		float64(size)/float64(divisor),
		"KMGTPE"[exponent],
	)
}
