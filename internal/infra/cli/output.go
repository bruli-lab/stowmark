package cli

import (
	"encoding/json"
	"io"
	"os"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/infra/progress"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	jsonFlag       = "json"
	noProgressFlag = "no-progress"
)

func addOutputFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool(jsonFlag, false, "print the result as JSON on stdout")
	cmd.PersistentFlags().Bool(noProgressFlag, false, "do not show the progress bar")
}

func progressFor(cmd *cobra.Command) app.Progress {
	disabled, err := cmd.Flags().GetBool(noProgressFlag)
	if err != nil || disabled || wantsJSON(cmd) {
		return nil
	}
	output, ok := cmd.ErrOrStderr().(*os.File)
	if !ok || !term.IsTerminal(int(output.Fd())) {
		return nil
	}
	return progress.NewTerminal(output)
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
