package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
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
			obsv, err := builtObservability(cmd.Context())
			if err != nil {
				return fmt.Errorf("create observability: %w", err)
			}
			repohand, err := repositories.NewHandler(cmd.Context(), repositoryPath)
			if err != nil {
				return err
			}
			defer func() {
				_ = obsv.Shutdown(cmd.Context())
				_ = repohand.Close()
			}()

			manifestRepo, err := repohand.ManifestRepository()
			if err != nil {
				return err
			}
			svc := snapshot.NewGetManifest(manifestRepo)
			tracerMdw := app.NewTracerQueryMiddleware(obsv.TracerProvider)
			handler := tracerMdw(app.NewManifestGet(svc))
			result, err := handler.Handle(cmd.Context(), app.ManifestGetQuery{
				SnapshotID: snapshotID,
			})
			manifest, ok := result.(*snapshot.Manifest)
			if !ok {
				return fmt.Errorf("unexpected result type: %T", result)
			}
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
