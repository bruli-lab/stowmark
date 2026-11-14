package cli

import (
	"errors"
	"fmt"
	"io"

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
		noIgnoreFile   bool
	)

	cmd := &cobra.Command{
		Use:   "create <source>",
		Short: "Create a snapshot of a directory",
		Long: "Create a snapshot of a directory. Files and directories that match the patterns of a " +
			snapshot.IgnoreFileName + " file, or an --exclude pattern, are not backed up. An ignore file applies to " +
			"its own directory and below and can override the ignore files of its parents. --exclude patterns are " +
			"applied last, so they can override every ignore file. --no-ignore-file backs up without reading them.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if repositoryPath == "" {
				return errors.New("--repo is required")
			}
			return withRepository(cmd, repositoryPath, func(repo *bootstrap.Repository) error {
				events, err := repo.CreateSnapshot().Handle(cmd.Context(), app.CreateSnapshotCommand{
					RepositoryPath:  repo.Path(),
					SourcePath:      args[0],
					PrivateKey:      optionalPath(privateKeyPath),
					Exclude:         exclude,
					SkipIgnoreFiles: noIgnoreFile,
				})
				if err != nil {
					return err
				}
				result, err := app.SingleEvent[*snapshot.SnapshotCreated](events)
				if err != nil {
					return err
				}
				return render(cmd, newCreateDocument(result), func(output io.Writer) error {
					if _, err := fmt.Fprintf(output, "Snapshot created: %s\nFiles: %d\nSize: %d bytes\n",
						result.SnapshotID(), result.FileCount(), result.TotalSize(),
					); err != nil {
						return err
					}
					for _, skipped := range result.SkippedFiles() {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "WARNING: %s skipped: %s is not supported\n", skipped.Path(), skipped.Kind())
					}
					return nil
				})
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
	cmd.Flags().BoolVar(
		&noIgnoreFile,
		"no-ignore-file",
		false,
		"do not read "+snapshot.IgnoreFileName+" files; only --exclude patterns apply",
	)
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}
