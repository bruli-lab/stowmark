package cli

import (
	"github.com/spf13/cobra"
)

func newSnapshotCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Manage repository snapshots",
	}

	cmd.AddCommand(
		newSnapshotCreateCommand(),
		newSnapshotListCommand(),
		newSnapshotGetCommand(),
	)

	return cmd
}
