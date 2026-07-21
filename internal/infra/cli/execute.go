package cli

import (
	"github.com/spf13/cobra"
)

func Execute() error {
	rootCmd := &cobra.Command{
		Use:           "stonekeep",
		Short:         "Immutable snapshot backup tool",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.AddCommand(newInitCommand())
	rootCmd.AddCommand(newSnapshotCommand())

	return rootCmd.Execute()
}
