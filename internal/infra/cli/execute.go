package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rootCmd := &cobra.Command{
		Use:           "stowmark",
		Short:         "Immutable snapshot backup tool",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.AddCommand(newInitCommand())
	rootCmd.AddCommand(newSnapshotCommand())
	rootCmd.AddCommand(newKeyCommand())

	return rootCmd.ExecuteContext(ctx)
}
