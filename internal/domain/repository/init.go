package repository

import (
	"context"
	"fmt"
)

type InitResult struct {
	ID       string
	Warnings []string
}
type Init struct {
	repo FolderRepository
}

func (i Init) Do(ctx context.Context, r *Repository, force bool) (*InitResult, error) {
	exists, err := i.repo.Exists(ctx, r.Path())
	if err != nil {
		return nil, err
	}
	result := &InitResult{}
	if err := i.repo.CreateFolder(ctx, r.Path()); err != nil {
		return nil, err
	}
	conf := r.Config()
	if exists {
		previous, err := i.repo.GetConfig(ctx, r.path)
		if err != nil {
			return nil, err
		}
		conf = updateConfig(previous, r.config, force, result)
	}
	if err := i.repo.CreateConfig(ctx, r.Path(), conf); err != nil {
		return nil, err
	}
	if err := i.repo.CreateFolder(ctx, r.ObjectsFolder()); err != nil {
		return nil, err
	}
	if err := i.repo.CreateFolder(ctx, r.SnapshotsFolder()); err != nil {
		return nil, err
	}
	result.ID = conf.Id().String()
	return result, nil
}

func NewInit(repo FolderRepository) *Init {
	return &Init{repo: repo}
}

type InitError struct {
	msg string
}

func (i InitError) Error() string {
	return fmt.Sprintf("init error: %s", i.msg)
}

func NewInitError(msg string) *InitError {
	return &InitError{msg: msg}
}
