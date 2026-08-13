package cli

import (
	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/webdav"
	"github.com/studio-b12/gowebdav"
)

type WebDavRepositories struct {
	client         *gowebdav.Client
	repositoryPath string
}

func (w WebDavRepositories) FolderRepository() repository.FolderRepository {
	return webdav.NewFolderRepositoryRepository(w.client)
}

func (w WebDavRepositories) RepositoryPath() string {
	return w.repositoryPath
}

func (w WebDavRepositories) ObjectRepository() (snapshot.ObjectRepository, error) {
	return webdav.NewObjectRepository(w.client, w.repositoryPath), nil
}

func (w WebDavRepositories) ManifestRepository() (snapshot.ManifestRepository, error) {
	return webdav.NewManifestRepository(w.client, w.repositoryPath), nil
}

func (w WebDavRepositories) Close() error {
	return nil
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
	return &WebDavRepositories{client: client, repositoryPath: location.Path}, nil
}
