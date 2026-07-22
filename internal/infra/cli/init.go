package cli

import (
	"fmt"

	"github.com/bruli-lab/stonekeep.git/internal/domain/repository"
	"github.com/bruli-lab/stonekeep.git/internal/infra/disk"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	repo := disk.NewFolderRepositoryRepository()
	svc := repository.NewInit(repo)
	return &cobra.Command{
		Use:   "init <repository>",
		Short: "Initialize a new Stonekeep repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := uuid.New()
			re, err := repository.NewRepository(args[0], repository.NewConfig(id, repository.NoneCompression()))
			if err != nil {
				return err
			}
			if err := svc.Do(cmd.Context(), re); err != nil {
				return err
			}
			fmt.Printf("Initialized Stonekeep repository at %q\n", args[0])
			fmt.Printf("Repository ID: %q", id.String())
			return nil
		},
	}
}
