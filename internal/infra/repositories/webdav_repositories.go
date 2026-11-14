package repositories

import (
	"github.com/bruli-lab/stowmark/internal/app"
	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/bruli-lab/stowmark/internal/infra/blob"
	"github.com/bruli-lab/stowmark/internal/infra/webdav"
	"github.com/studio-b12/gowebdav"
)

type WebDavRepositories struct {
	client         *gowebdav.Client
	store          *webdav.BlobStore
	repositoryPath string
}

func (w WebDavRepositories) FolderRepository() app.FolderRepository {
	return webdav.NewFolderRepositoryRepository(w.client)
}

func (w WebDavRepositories) RepositoryPath() string {
	return w.repositoryPath
}

func (w WebDavRepositories) Close() error {
	return nil
}

func (w WebDavRepositories) BlobStore() (blob.Store, error) {
	return w.store, nil
}

func NewWebDavRepositories(address string) (*WebDavRepositories, error) {
	conf, err := config.NewWebdavConfig()
	if err != nil {
		return nil, err
	}
	location, err := webdav.ParseWebDAVURL(address)
	if err != nil {
		return nil, err
	}
	client, err := webdav.NewClient(location.BaseURL, conf.Username, conf.Password)
	if err != nil {
		return nil, err
	}
	return &WebDavRepositories{client: client, store: webdav.NewBlobStore(client, location.Path), repositoryPath: location.Path}, nil
}
