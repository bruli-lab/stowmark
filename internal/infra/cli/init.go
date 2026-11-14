package cli

import (
	"fmt"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	var (
		repositoryPath  string
		compressionType string
		level           int
		formatVersion   int
		publicKey       string
		force           bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new Stowmark repository",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			obsv, err := buildObservability(cmd.Context())
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
			compType, err := repository.ParseCompressionType(compressionType)
			if err != nil {
				return fmt.Errorf("invalid compression type: %w", err)
			}

			var compressionLevel *int
			if cmd.Flags().Changed("level") {
				compressionLevel = &level
			}

			comp, err := repository.NewCompression(*compType, compressionLevel)
			if err != nil {
				return fmt.Errorf("create compression configuration: %w", err)
			}

			var publicKeyPath *string
			if publicKey != "" {
				publicKeyPath = &publicKey
			}

			version, err := repository.ParseFormatVersionOrCurrent(formatVersion)
			if err != nil {
				return err
			}

			evtr := app.NewEventsTracing()
			multiMdw, err := middlewares.BuildCommandMiddlewares(obsv, evtr)
			if err != nil {
				return err
			}
			locker, err := newRepositoryLocker(repoHand)
			if err != nil {
				return err
			}
			ch := multiMdw(app.NewInit(repoHand.FolderRepository(), disk.NewAsymmetricKeyPairRepository(), encrypt.NewSymmetricRepository(), locker))

			events, err := ch.Handle(cmd.Context(), app.InitCommand{
				RepositoryPath: repoHand.RepositoryPath(),
				Compression:    comp,
				FormatVersion:  version,
				PublicKeyPath:  publicKeyPath,
				Force:          force,
			})
			if err != nil {
				return fmt.Errorf("initialize repository: %w", err)
			}
			result, err := app.SingleEvent[*repository.RepositoryInitialized](events)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(
				cmd.OutOrStdout(),
				"Initialized Stowmark repository at %q\n",
				repoHand.RepositoryPath(),
			)
			_, _ = fmt.Fprintf(
				cmd.OutOrStdout(),
				"Repository ID: %s\n",
				result.RepositoryID(),
			)
			for _, warning := range result.Warnings() {
				_, _ = fmt.Fprintf(
					cmd.OutOrStdout(),
					"WARNING: %s\n",
					warning,
				)
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
		&compressionType,
		"compression",
		repository.NoneCompressionType.String(),
		"compression type: none, gzip, zstd, lz4, xz",
	)

	cmd.Flags().StringVar(
		&publicKey,
		"public-key",
		"",
		"path to the public key used to encrypt the repository",
	)

	cmd.Flags().IntVar(
		&formatVersion,
		"version",
		0,
		"repository format version (1 or 2); defaults to the current version",
	)

	cmd.Flags().IntVar(
		&level,
		"level",
		0,
		"compression level (gzip 1-9, zstd 1-22, lz4 0-9); defaults to the algorithm default",
	)
	cmd.Flags().BoolVar(
		&force,
		"force",
		false,
		"Force the operation",
	)

	_ = cmd.MarkFlagRequired("repo")

	return cmd
}
