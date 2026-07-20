package cli

import (
	"context"

	"github.com/spf13/cobra"
)

func Execute(ctx context.Context) error {
	rootCmd := &cobra.Command{
		Use:           "stonekeep",
		Short:         "Immutable snapshot backup tool",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.AddCommand(newInitCommand(ctx))

	return rootCmd.Execute()
}
