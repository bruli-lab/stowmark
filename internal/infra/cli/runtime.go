package cli

import (
	"context"
	"errors"

	"github.com/bruli-lab/stowmark/internal/bootstrap"
	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/spf13/cobra"
)

func withRuntime(cmd *cobra.Command, run func(runtime *bootstrap.Runtime) error) (err error) {
	runtime, err := bootstrap.NewRuntime(cmd.Context(), config.ProcessEnvironment())
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, runtime.Close(context.WithoutCancel(cmd.Context())))
	}()
	return run(runtime)
}

func withRepository(cmd *cobra.Command, repositoryURL string, run func(repo *bootstrap.Repository) error) error {
	return withRuntime(cmd, func(runtime *bootstrap.Runtime) (err error) {
		repo, err := runtime.OpenRepository(cmd.Context(), repositoryURL)
		if err != nil {
			return err
		}
		defer func() {
			err = errors.Join(err, repo.Close())
		}()
		return run(repo)
	})
}

func optionalPath(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
