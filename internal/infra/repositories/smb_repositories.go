package repositories

import (
	"context"

	"github.com/bruli-lab/stowmark/internal/infra/blob"

	"github.com/bruli-lab/stowmark/internal/config"
	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/smb"
	"github.com/cloudsoda/go-smb2"
)

type SmbRepositories struct {
	repositoryPath string
	share          *smb2.Share
	conn           *smb.Connection
}

func (s SmbRepositories) Close() error {
	return s.conn.Close()
}

func (s SmbRepositories) FolderRepository() repository.FolderRepository {
	return smb.NewFolderRepositoryRepository(s.share)
}

func (s SmbRepositories) RepositoryPath() string {
	return s.repositoryPath
}

func (s SmbRepositories) BlobStore() (blob.Store, error) {
	return smb.NewBlobStore(s.share, s.repositoryPath), nil
}

func NewSmbRepositories(ctx context.Context, repositoryPath string) (*SmbRepositories, error) {
	conf, err := config.NewSMBConfig()
	if err != nil {
		return nil, err
	}
	repoURL, err := smb.ParseRepositoryURL(repositoryPath)
	if err != nil {
		return nil, err
	}
	conn, err := smb.Connect(ctx, repoURL.Address, repoURL.Username, conf.Password, repoURL.ShareName)
	if err != nil {
		return nil, err
	}
	return &SmbRepositories{repositoryPath: repoURL.BasePath, share: conn.Share, conn: conn}, nil
}
