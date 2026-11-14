package cli

import (
	"errors"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/bootstrap"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/spf13/cobra"
)

func newSnapshotCreateCommand() *cobra.Command {
	var (
		repositoryPath string
		privateKeyPath string
		exclude        []string
	)

	cmd := &cobra.Command{
		Use:   "create <source>",
		Short: "Create a snapshot of a directory",
		Long: "Create a snapshot of a directory. Files and directories that match the patterns in the " +
			snapshot.IgnoreFileName + " file at the root of the source, or an --exclude pattern, are not backed up. " +
			"--exclude patterns are applied after the ignore file, so they can override it.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if repositoryPath == "" {
				return errors.New("--repo is required")
			}
			return withRepository(cmd, repositoryPath, func(repo *bootstrap.Repository) error {
				events, err := repo.CreateSnapshot().Handle(cmd.Context(), app.CreateSnapshotCommand{
					RepositoryPath: repo.Path(),
					SourcePath:     args[0],
					PrivateKey:     optionalPath(privateKeyPath),
					Exclude:        exclude,
				})
				if err != nil {
					return err
				}
				result, err := app.SingleEvent[*snapshot.SnapshotCreated](events)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(
					cmd.OutOrStdout(),
					"Snapshot created: %s\nFiles: %d\nSize: %d bytes\n",
					result.SnapshotID(),
					result.FileCount(),
					result.TotalSize(),
				)
				for _, skipped := range result.SkippedFiles() {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "WARNING: %s skipped: %s is not supported\n", skipped.Path(), skipped.Kind())
				}
				return nil
			})
		},
	}

	cmd.Flags().StringVar(
		&repositoryPath,
		"repo",
		"",
		"path to the Stowmark repository",
	)

	cmd.Flags().StringVar(
		&privateKeyPath,
		"private-key",
		"",
		"path to the private key for encryption",
	)
	cmd.Flags().StringArrayVar(
		&exclude,
		"exclude",
		nil,
		"exclude files and directories matching this pattern (repeatable, same syntax as "+snapshot.IgnoreFileName+")",
	)
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}
