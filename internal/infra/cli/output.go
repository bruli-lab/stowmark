package cli

import (
	"encoding/json"
	"io"

	"github.com/spf13/cobra"
)

const jsonFlag = "json"

func addJSONFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool(jsonFlag, false, "print the result as JSON on stdout")
}

func wantsJSON(cmd *cobra.Command) bool {
	enabled, err := cmd.Flags().GetBool(jsonFlag)
	return err == nil && enabled
}

func render(cmd *cobra.Command, document any, text func(output io.Writer) error) error {
	if !wantsJSON(cmd) {
		return text(cmd.OutOrStdout())
	}
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}
