package repository

import (
	"context"
	"fmt"
)

type Init struct {
	repo FolderRepository
}

func (i Init) Do(ctx context.Context, r *Repository) error {
	ex, err := i.repo.Exists(ctx, r.Path())
	if err != nil {
		return err
	}
	if err := i.repo.CreateFolder(ctx, r.Path()); err != nil {
		return err
	}
	conf := r.Config()
	if ex {
		previous, err := i.repo.GetConfig(ctx, r.path)
		if err != nil {
			return err
		}
		conf = NewConfig(previous.Id(), previous.formatVersion, r.config.compression)
	}
	if err := i.repo.CreateConfig(ctx, r.Path(), conf); err != nil {
		return err
	}
	if err := i.repo.CreateFolder(ctx, r.ObjectsFolder()); err != nil {
		return err
	}
	if err := i.repo.CreateFolder(ctx, r.SnapshotsFolder()); err != nil {
		return err
	}
	return nil
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
