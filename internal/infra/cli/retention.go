package cli

import (
	"fmt"
	"io"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/bootstrap"
	"github.com/spf13/cobra"
)

func newRetentionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "retention",
		Short: "Manage the object lock retention of a repository",
	}
	cmd.AddCommand(newRetentionExtendCommand())
	return cmd
}

func newRetentionExtendCommand() *cobra.Command {
	var (
		repositoryPath string
		privateKeyPath string
	)

	cmd := &cobra.Command{
		Use:   "extend",
		Short: "Extend the object lock retention of everything the snapshots use",
		Long: "Extend the S3 Object Lock or GCS object retention of the config, every snapshot manifest and every object they " +
			"reference to one full period from now, for the items whose retention ends within half a period. " +
			"Objects written once and reused by later snapshots keep their first retention, so run it regularly " +
			"(for example from cron, more often than half the period).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withRepository(cmd, repositoryPath, func(repo *bootstrap.Repository) error {
				events, err := repo.ExtendRetention().Handle(cmd.Context(), app.ExtendRetentionCommand{
					RepositoryPath: repo.Path(),
					PrivateKeyPath: optionalPath(privateKeyPath),
					Progress:       progressFor(cmd),
				})
				if err != nil {
					return fmt.Errorf("failed to extend the retention: %w", err)
				}
				document := newRetentionExtendDocument(events)
				return render(cmd, document, func(output io.Writer) error {
					if document.RetainedUntil == nil {
						_, err := fmt.Fprintln(output, "Every item is retained for more than half a period; nothing to extend")
						return err
					}
					_, err := fmt.Fprintf(output, "Extended the retention of %d item(s) until %s\n", document.Extended, formatLocal(*document.RetainedUntil))
					return err
				})
			})
		},
	}

	cmd.Flags().StringVar(&repositoryPath, "repo", "", "path to the Stowmark repository")
	cmd.Flags().StringVar(&privateKeyPath, "private-key", "", metadataPrivateKeyUsage)
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}
