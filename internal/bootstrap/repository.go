package bootstrap

import (
	"context"
	"errors"

	"github.com/bruli-lab/go-core/cqs"
	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/infra/repositories"
)

type Repository struct {
	runtime   *Runtime
	backend   repositories.Repositories
	folder    app.FolderRepository
	manifests app.ManifestRepository
	objects   app.ObjectRepository
	encrypted app.EncryptedObjectRepository
	locks     app.LockRepository
	locker    *app.RepositoryLocker
}

func (r *Runtime) OpenRepository(ctx context.Context, repositoryURL string) (*Repository, error) {
	backend, err := repositories.NewHandler(ctx, repositoryURL, r.environment)
	if err != nil {
		return nil, err
	}
	repo := &Repository{runtime: r, backend: backend, folder: backend.FolderRepository()}
	if err := repo.open(); err != nil {
		return nil, errors.Join(err, backend.Close())
	}
	return repo, nil
}

func (r *Repository) open() error {
	var err error
	if r.manifests, err = r.backend.ManifestRepository(); err != nil {
		return err
	}
	if r.objects, err = r.backend.ObjectRepository(); err != nil {
		return err
	}
	if r.encrypted, err = r.backend.EncryptedObjectRepository(); err != nil {
		return err
	}
	if r.locks, err = r.backend.LockRepository(); err != nil {
		return err
	}
	r.locker = app.NewRepositoryLocker(r.locks)
	return nil
}

func (r *Repository) Path() string {
	return r.backend.RepositoryPath()
}

func (r *Repository) Close() error {
	return r.backend.Close()
}

func (r *Repository) Init() cqs.CommandHandler {
	return r.runtime.commands(app.NewInit(r.folder, r.runtime.asymmetric, r.runtime.symmetric, r.locker))
}

func (r *Repository) CreateSnapshot() cqs.CommandHandler {
	return r.runtime.commands(app.NewCreateSnapshot(
		r.runtime.source, r.manifests, r.objects, r.folder, r.runtime.asymmetric, r.runtime.symmetric, r.locker,
	))
}

func (r *Repository) RestoreSnapshot() cqs.CommandHandler {
	return r.runtime.commands(app.NewRestoreSnapshot(
		r.manifests, r.objects, r.folder, r.runtime.asymmetric, r.runtime.symmetric, r.locker,
	))
}

func (r *Repository) RestoreFile() cqs.CommandHandler {
	return r.runtime.commands(app.NewRestoreFile(
		r.manifests, r.objects, r.folder, r.runtime.asymmetric, r.runtime.symmetric, r.locker,
	))
}

func (r *Repository) RekeyKey() cqs.CommandHandler {
	return r.runtime.commands(app.NewRekeyKey(r.folder, r.encrypted, r.runtime.asymmetric, r.runtime.symmetric, r.locker))
}

func (r *Repository) RewrapKey() cqs.CommandHandler {
	return r.runtime.commands(app.NewRewrapKey(r.folder, r.runtime.asymmetric, r.runtime.symmetric, r.locker))
}

func (r *Repository) Unlock() cqs.CommandHandler {
	return r.runtime.commands(app.NewUnlock(r.locks))
}

func (r *Repository) ForgetSnapshots() cqs.CommandHandler {
	return r.runtime.commands(app.NewForgetSnapshots(r.manifests, r.locker))
}

func (r *Repository) PruneRepository() cqs.CommandHandler {
	return r.runtime.commands(app.NewPruneRepository(r.folder, r.manifests, r.objects, r.encrypted, r.locker))
}

func (r *Repository) SnapshotList() cqs.QueryHandler {
	return r.runtime.queries(app.NewSnapshotList(r.manifests))
}

func (r *Repository) ManifestGet() cqs.QueryHandler {
	return r.runtime.queries(app.NewManifestGet(r.manifests))
}

func (r *Repository) VerifySnapshot() cqs.QueryHandler {
	return r.runtime.queries(app.NewVerifySnapshot(
		r.manifests, r.objects, r.folder, r.runtime.asymmetric, r.runtime.symmetric, r.locker,
	))
}

func (r *Repository) CheckRepository() cqs.QueryHandler {
	return r.runtime.queries(app.NewCheckRepository(
		r.folder, r.manifests, r.objects, r.encrypted, r.locks, r.runtime.asymmetric, r.runtime.symmetric, r.locker,
	))
}

func (r *Repository) ForgetPlan() cqs.QueryHandler {
	return r.runtime.queries(app.NewForgetPlan(r.manifests, r.locker))
}

func (r *Repository) PrunePlan() cqs.QueryHandler {
	return r.runtime.queries(app.NewPrunePlan(r.folder, r.manifests, r.objects, r.encrypted, r.locker))
}

func (r *Repository) LockList() cqs.QueryHandler {
	return r.runtime.queries(app.NewLockList(r.locks))
}
