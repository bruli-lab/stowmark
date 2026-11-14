package cli

import (
	"errors"
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
	"github.com/spf13/cobra"
)

func newSnapshotCreateCommand() *cobra.Command {
	var (
		repositoryPath string
		privateKeyPath string
		exclude        []string
	)
	sourceRepo := disk.NewSourceRepository()

	cmd := &cobra.Command{
		Use:   "create <source>",
		Short: "Create a snapshot of a directory",
		Long: "Create a snapshot of a directory. Files and directories that match the patterns in the " +
			snapshot.IgnoreFileName + " file at the root of the source, or an --exclude pattern, are not backed up. " +
			"--exclude patterns are applied after the ignore file, so they can override it.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourcePath := args[0]

			if repositoryPath == "" {
				return errors.New("--repo is required")
			}

			repoHand, err := repositories.NewHandler(cmd.Context(), repositoryPath)
			if err != nil {
				return err
			}
			defer func() {
				_ = repoHand.Close()
			}()

			manifestRepo, err := repoHand.ManifestRepository()
			if err != nil {
				return err
			}
			objRepo, err := repoHand.ObjectRepository()
			if err != nil {
				return err
			}
			obsv, err := buildObservability(cmd.Context())
			if err != nil {
				return err
			}
			defer func() {
				_ = obsv.Shutdown(cmd.Context())
			}()
			evTr := app.NewEventsTracing()
			evTr.Add(snapshot.SnapshotCreatedEventName, app.NewCreateSnapshotTracer(obsv.Logger))
			multiMdw, err := middlewares.BuildCommandMiddlewares(obsv, evTr)
			if err != nil {
				return err
			}
			locker, err := newRepositoryLocker(repoHand)
			if err != nil {
				return err
			}
			handler := multiMdw(app.NewCreateSnapshot(
				sourceRepo,
				manifestRepo,
				objRepo,
				repoHand.FolderRepository(),
				disk.NewAsymmetricKeyPairRepository(),
				encrypt.NewSymmetricRepository(),
				locker,
			))

			var privateKey *string
			if privateKeyPath != "" {
				privateKey = &privateKeyPath
			}

			events, err := handler.Handle(cmd.Context(), app.CreateSnapshotCommand{
				RepositoryPath: repoHand.RepositoryPath(),
				SourcePath:     sourcePath,
				PrivateKey:     privateKey,
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
