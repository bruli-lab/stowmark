package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var version = "dev"

func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rootCmd := &cobra.Command{
		Use:           "stowmark",
		Short:         "Immutable snapshot backup tool",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.AddCommand(newInitCommand())
	rootCmd.AddCommand(newSnapshotCommand())
	rootCmd.AddCommand(newKeyCommand())
	rootCmd.AddCommand(newUnlockCommand())
	rootCmd.AddCommand(newForgetCommand())
	rootCmd.AddCommand(newPruneCommand())
	rootCmd.AddCommand(newCheckCommand())

	return rootCmd.ExecuteContext(ctx)
}
