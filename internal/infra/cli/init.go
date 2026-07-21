package cli

import (
	"fmt"
	"time"

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
			re, err := repository.NewRepository(args[0], repository.NewConfig(id, time.Now()))
			if err != nil {
				return err
			}
			if err := svc.Do(cmd.Context(), re); err != nil {
				return err
			}
			fmt.Println(fmt.Sprintf("Initialized Stonekeep repository at %q", args[0]))
			fmt.Println(fmt.Sprintf("Repository ID: %q", id.String()))
			return nil
		},
	}
}
