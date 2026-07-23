package cli

import (
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/bruli-lab/stowmark.git/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark.git/internal/infra/disk"
	"github.com/spf13/cobra"
)

func newSnapshotGetCommand() *cobra.Command {
	var (
		repositoryPath string
		snapshotID     string
	)
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Show a snapshot manifest",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if repositoryPath == "" {
				return errors.New("--repo is required")
			}
			if snapshotID == "" {
				return errors.New("--id is required")
			}

			manifestRepo, err := disk.NewManifestRepository(repositoryPath)
			if err != nil {
				return err
			}
			get := snapshot.NewGetManifest(manifestRepo)
			manifest, err := get.Get(cmd.Context(), snapshotID)
			if err != nil {
				return err
			}
			return printManifest(cmd.OutOrStdout(), manifest)
		},
	}

	cmd.Flags().StringVar(
		&repositoryPath,
		"repo",
		"",
		"path to the Stowmark repository",
	)
	cmd.Flags().StringVar(
		&snapshotID,
		"id",
		"",
		"snapshot ID",
	)

	return cmd
}

func printManifest(
	output io.Writer,
	manifest *snapshot.Manifest,
) error {
	writer := tabwriter.NewWriter(
		output,
		0, // amplada mínima
		4, // amplada del tabulador
		2, // espais entre columnes
		' ',
		0,
	)

	createdAt := manifest.CreatedAt().Local()

	_, err := fmt.Fprintf(
		writer,
		"ID:\t%s\n"+
			"CREATED AT:\t%s\n"+
			"SOURCE:\t%s\n",
		manifest.Id(),
		createdAt.Format("2006-01-02 15:04:05"),
		manifest.Source(),
	)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintln(
		writer,
		"\nPATH\tHASH\tSIZE",
	); err != nil {
		return err
	}

	for _, file := range manifest.Files() {
		if _, err := fmt.Fprintf(
			writer,
			"%s\t%s\t%s\n",
			file.Path(),
			file.Hash(),
			formatBytes(file.Size()),
		); err != nil {
			return err
		}
	}

	return writer.Flush()
}
