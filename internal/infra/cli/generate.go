package cli

import (
	"fmt"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/encryption"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/spf13/cobra"
)

func newKeyGenerateCommand() *cobra.Command {
	var folder string
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Crate a new encryption key pair",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			obsv, err := builtObservability(cmd.Context())
			if err != nil {
				return fmt.Errorf("create observability: %w", err)
			}
			defer func() {
				_ = obsv.Shutdown(cmd.Context())
			}()
			repo := disk.NewAsymmetricKeyPairRepository()
			svc := encryption.NewCreateAsymmetricKeyPair(repo)
			loggerMdw := app.NewLoggerCommandMiddleware(obsv.Logger)
			tracerMdw := app.NewTracerCommandMiddleware(obsv.TracerProvider)
			multiMdw := cqs.CommandHandlerMultiMiddleware(loggerMdw, tracerMdw)
			handler := multiMdw(app.NewGenerateKey(svc))
			keys, err := encryption.NewAsymmetricKeyPair(folder)
			if err != nil {
				return fmt.Errorf("failed to create key pair: %w", err)
			}
			_, err = handler.Handle(cmd.Context(), app.GenerateKeyCommand{Keys: keys})
			if err != nil {
				return fmt.Errorf("failed to create key pair: %w", err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Create asymmetric key pair in %q\n", folder)
			return err
		},
	}

	cmd.Flags().StringVar(
		&folder,
		"folder",
		"",
		"Folder to generate key pair",
	)

	_ = cmd.MarkFlagRequired("folder")

	return cmd
}
