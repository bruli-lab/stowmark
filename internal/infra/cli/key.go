package cli

import (
	"github.com/spf13/cobra"
)

func newKeyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key",
		Short: "Manage encryption keys",
	}

	cmd.AddCommand(newKeyGenerateCommand())
	cmd.AddCommand(newKeyRewrapCommand())
	cmd.AddCommand(newKeyReKeyCommand())

	return cmd
}
