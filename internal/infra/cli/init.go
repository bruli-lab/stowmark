package cli

import (
	"fmt"

	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
	"github.com/google/uuid"
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
			repoHand, err := repositories.NewHandler(cmd.Context(), repositoryPath)
			if err != nil {
				return err
			}
			defer func() {
				_ = repoHand.Close()
			}()
			svc := repository.NewInit(repoHand.FolderRepository())
			compType, err := repository.ParseCompressionType(compressionType)
			if err != nil {
				return fmt.Errorf("invalid compression type: %w", err)
			}

			comp, err := repository.NewCompression(*compType, &level)
			if err != nil {
				return fmt.Errorf("create compression configuration: %w", err)
			}

			id := uuid.New()

			var encryptionConfig *encryption.EncryptionConfig

			if publicKey != "" {
				asymmetricRepo := disk.NewAsymmetricKeyPairRepository()
				symmetricRepo := encrypt.NewSymmetricRepository()
				encryptionConfigSvc := encryption.NewCreateEncryptionConfig(asymmetricRepo, symmetricRepo)
				encryptionConfig, err = encryptionConfigSvc.Create(cmd.Context(), publicKey)
				if err != nil {
					return fmt.Errorf("create encryption config: %w", err)
				}
			}

			re, err := repository.NewRepository(
				repoHand.RepositoryPath(),
				repository.NewConfig(id, repository.ParseFormatVersion(formatVersion), comp, encryptionConfig),
			)
			if err != nil {
				return fmt.Errorf("create repository: %w", err)
			}

			result, err := svc.Do(cmd.Context(), re, force)
			if err != nil {
				return fmt.Errorf("initialize repository: %w", err)
			}

			_, _ = fmt.Fprintf(
				cmd.OutOrStdout(),
				"Initialized Stowmark repository at %q\n",
				repoHand.RepositoryPath(),
			)
			_, _ = fmt.Fprintf(
				cmd.OutOrStdout(),
				"Repository ID: %s\n",
				result.ID,
			)
			for _, warning := range result.Warnings {
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
		"compression type: none, zstd",
	)

	cmd.Flags().StringVar(
		&publicKey,
		"public-key",
		"",
		"public key for encryption:",
	)

	cmd.Flags().IntVar(
		&formatVersion,
		"version",
		0,
		"format version for config file",
	)

	cmd.Flags().IntVar(
		&level,
		"level",
		0,
		"compression level: 0 for none, 1-22 for zstd",
	)
	cmd.Flags().BoolVar(
		&force,
		"force",
		false,
		"Force the operation",
	)

	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("compression")

	return cmd
}
