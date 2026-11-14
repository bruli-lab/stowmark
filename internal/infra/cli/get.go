package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/bootstrap"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/spf13/cobra"
)

func newSnapshotGetCommand() *cobra.Command {
	var (
		repositoryPath string
		snapshotID     string
		privateKeyPath string
	)
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Show a snapshot manifest",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withRepository(cmd, repositoryPath, func(repo *bootstrap.Repository) error {
				manifest, err := app.QueryResult[*snapshot.Manifest](repo.ManifestGet().Handle(cmd.Context(), app.ManifestGetQuery{
					RepositoryPath: repo.Path(),
					PrivateKeyPath: optionalPath(privateKeyPath),
					SnapshotID:     snapshotID,
				}))
				if err != nil {
					return err
				}
				return render(cmd, newManifestDocument(manifest), func(output io.Writer) error {
					return printManifest(output, manifest)
				})
			})
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
	cmd.Flags().StringVar(&privateKeyPath, "private-key", "", metadataPrivateKeyUsage)

	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}

func printManifest(
	output io.Writer,
	manifest *snapshot.Manifest,
) error {
	writer := tabwriter.NewWriter(
		output,
		0,
		4,
		2,
		' ',
		0,
	)

	createdAt := manifest.CreatedAt().Local()

	_, err := fmt.Fprintf(
		writer,
		"ID:\t%s\n"+
			"CREATED AT:\t%s\n"+
			"SOURCE:\t%s\n",
		manifest.ID(),
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
			entryName(file),
			file.Hash(),
			formatBytes(file.Size()),
		); err != nil {
			return err
		}
	}

	return writer.Flush()
}

func entryName(file snapshot.File) string {
	switch file.Kind() {
	case snapshot.DirectoryEntry:
		return file.Path() + "/"
	case snapshot.SymlinkEntry:
		return file.Path() + " -> " + file.Target()
	default:
		return file.Path()
	}
}
