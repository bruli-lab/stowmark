package cli

import (
	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
)

func newRepositoryLocker(repos repositories.Repositories) (*app.RepositoryLocker, error) {
	lockRepo, err := repos.LockRepository()
	if err != nil {
		return nil, err
	}
	return app.NewRepositoryLocker(lockRepo), nil
}
