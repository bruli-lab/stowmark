package cli

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/disk"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/middlewares"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
	"github.com/spf13/cobra"
)

var ErrRepositoryCheckFailed = errors.New("repository check found errors")

func newCheckCommand() *cobra.Command {
	var (
		repositoryPath string
		readData       bool
		subsetValue    string
		privateKeyPath string
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check the integrity of the whole repository",
		Long: "Check that every manifest can be read and that every object it references exists. " +
			"With --read-data, also download every referenced object once and verify its hash; " +
			"--read-data-subset verifies only a part of them (for example 10% or 2/7). " +
			"Reading the data of an encrypted repository needs --private-key.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var subset *snapshot.DataSubset
			if subsetValue != "" {
				parsed, err := snapshot.ParseDataSubset(subsetValue)
				if err != nil {
					return err
				}
				subset = parsed
			}
			var privateKey *string
			if privateKeyPath != "" {
				privateKey = &privateKeyPath
			}

			obsv, err := buildObservability(cmd.Context())
			if err != nil {
				return fmt.Errorf("create observability: %w", err)
			}
			repoHandler, err := repositories.NewHandler(cmd.Context(), repositoryPath)
			if err != nil {
				return err
			}
			defer func() {
				_ = obsv.Shutdown(cmd.Context())
				_ = repoHandler.Close()
			}()
			manifestRepo, err := repoHandler.ManifestRepository()
			if err != nil {
				return err
			}
			objectRepo, err := repoHandler.ObjectRepository()
			if err != nil {
				return err
			}
			encryptedRepo, err := repoHandler.EncryptedObjectRepository()
			if err != nil {
				return err
			}
			lockRepo, err := repoHandler.LockRepository()
			if err != nil {
				return err
			}
			locker, err := newRepositoryLocker(repoHandler)
			if err != nil {
				return fmt.Errorf("failed to create repository locker: %w", err)
			}
			queryMdw, err := middlewares.BuildQueryMiddlewares(obsv)
			if err != nil {
				return err
			}
			handler := queryMdw(app.NewCheckRepository(
				repoHandler.FolderRepository(), manifestRepo, objectRepo, encryptedRepo, lockRepo,
				disk.NewAsymmetricKeyPairRepository(), encrypt.NewSymmetricRepository(), locker,
			))
			result, err := handler.Handle(cmd.Context(), app.CheckRepositoryQuery{
				RepositoryPath: repoHandler.RepositoryPath(),
				ReadData:       readData,
				Subset:         subset,
				PrivateKeyPath: privateKey,
			})
			if err != nil {
				return fmt.Errorf("failed to check repository: %w", err)
			}
			report, ok := result.(*snapshot.CheckReport)
			if !ok {
				return fmt.Errorf("unexpected result type: %T", result)
			}
			if err := printCheckReport(cmd.OutOrStdout(), report); err != nil {
				return err
			}
			if report.HasErrors() {
				return ErrRepositoryCheckFailed
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repositoryPath, "repo", "", "path to the Stowmark repository")
	cmd.Flags().BoolVar(&readData, "read-data", false, "download every referenced object once and verify its hash")
	cmd.Flags().StringVar(&subsetValue, "read-data-subset", "", "verify only a subset of the objects: a percentage (10%) or a part (2/7)")
	cmd.Flags().StringVar(&privateKeyPath, "private-key", "", "private key, needed to read the data of an encrypted repository")
	cmd.MarkFlagsMutuallyExclusive("read-data", "read-data-subset")
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}

func printCheckReport(output io.Writer, report *snapshot.CheckReport) error {
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	rows := [][2]string{
		{"SNAPSHOTS:", strconv.Itoa(report.Snapshots())},
		{"CORRUPT MANIFESTS:", strconv.Itoa(len(report.CorruptManifests()))},
		{"STORED OBJECTS:", strconv.Itoa(report.StoredObjects())},
		{"REFERENCED OBJECTS:", strconv.Itoa(report.ReferencedObjects())},
		{"MISSING OBJECTS:", strconv.Itoa(len(report.MissingObjects()))},
		{"UNREFERENCED OBJECTS:", strconv.Itoa(report.UnreferencedObjects())},
		{"STALE LOCKS:", strconv.Itoa(report.StaleLocks())},
		{"OLD KEY GENERATIONS:", strconv.Itoa(len(report.OrphanGenerations()))},
	}
	if report.ReadData() {
		rows = append(rows,
			[2]string{"READ OBJECTS:", strconv.Itoa(report.ReadObjects())},
			[2]string{"CORRUPT OBJECTS:", strconv.Itoa(len(report.CorruptObjects()))},
		)
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(writer, "%s\t%s\n", row[0], row[1]); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}

	lines := make([]string, 0)
	for _, problem := range report.CorruptManifests() {
		lines = append(lines, fmt.Sprintf("ERROR: manifest of snapshot %s cannot be read: %s", problem.SnapshotID(), problem.Reason()))
	}
	for _, missing := range report.MissingObjects() {
		lines = append(lines, fmt.Sprintf("ERROR: object %s is missing, used by snapshots %s", missing.Hash(), strings.Join(missing.Snapshots(), ", ")))
	}
	for _, problem := range report.CorruptObjects() {
		lines = append(lines, fmt.Sprintf("ERROR: object %s is corrupt: %s", problem.Hash(), problem.Reason()))
	}
	if report.UnreferencedObjects() > 0 {
		lines = append(lines, fmt.Sprintf("WARNING: %d object(s) are not used by any snapshot; run prune to remove them", report.UnreferencedObjects()))
	}
	if report.StaleLocks() > 0 {
		lines = append(lines, fmt.Sprintf("WARNING: %d stale lock(s); run unlock to remove them", report.StaleLocks()))
	}
	if generations := report.OrphanGenerations(); len(generations) > 0 {
		lines = append(lines, fmt.Sprintf("WARNING: objects of old key generation(s) %s remain, probably from an interrupted key rekey; run prune to remove them", formatGenerations(generations)))
	}
	status := "Repository OK"
	if report.HasErrors() {
		status = "Repository has errors"
	}
	lines = append(lines, status)

	_, err := fmt.Fprintf(output, "\n%s\n", strings.Join(lines, "\n"))
	return err
}
