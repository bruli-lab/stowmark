package app

import (
	"context"
	"crypto/rsa"
	"io"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/google/uuid"
)

//go:generate go tool moq -out repositories_mock.go . FolderRepository SourceRepository ManifestRepository ObjectRepository AsymmetricKeyPairRepository SymmetricKeyRepository LockRepository
type FolderRepository interface {
	Exists(ctx context.Context, path string) (bool, error)
	CreateFolder(ctx context.Context, path string) error
	CreateConfig(ctx context.Context, path string, c *repository.Config) error
	GetConfig(ctx context.Context, path string) (*repository.Config, error)
}

type SourceRepository interface {
	IgnorePatterns(ctx context.Context, sourcePath string) ([]string, error)
	Explore(ctx context.Context, sourcePath string, rules *snapshot.ExcludeRules) (*snapshot.Source, error)
	CalculateHash(ctx context.Context, filePath string, comp *repository.Compression) (string, error)
	CalculateChunks(ctx context.Context, filePath string, size int64, comp *repository.Compression) ([]snapshot.Chunk, error)
}

type ManifestRepository interface {
	Save(ctx context.Context, m *snapshot.Manifest) error
	List(ctx context.Context) ([]snapshot.Manifest, error)
	ListIDs(ctx context.Context) ([]string, error)
	Get(ctx context.Context, snapshotID string) (*snapshot.Manifest, error)
	Delete(ctx context.Context, snapshotID string) error
}

type ObjectRepository interface {
	Save(ctx context.Context, filePath, hash string, comp *repository.Compression, symmetricKey []byte, generation uint64) error
	AlreadyExists(ctx context.Context, hash string, symmetricKey []byte, generation uint64) (bool, error)
	ReadObject(ctx context.Context, hash string, symmetricKey []byte, generation uint64) (io.ReadCloser, error)
	RestoreObject(ctx context.Context, comp *repository.Compression, obj *snapshot.File, symmetricKey []byte, generation uint64) error
	SaveChunk(ctx context.Context, filePath string, hash string, offset int64, size int64, comp *repository.Compression, key []byte, generation uint64) error
	ListObjects(ctx context.Context, encrypted bool, generation uint64) ([]string, error)
	DeleteObject(ctx context.Context, hash string, encrypted bool, generation uint64) error
	ListEncryptedObjects(ctx context.Context, generation uint64) ([]string, error)
	ListEncryptedGenerations(ctx context.Context) ([]uint64, error)
	ReadEncryptedObject(ctx context.Context, hash string, generation uint64, key []byte) (io.ReadCloser, error)
	SaveRekeyedObject(ctx context.Context, hash string, source io.Reader, generation uint64, key []byte) error
	DeleteEncryptedGeneration(ctx context.Context, generation uint64) error
	AbortRekey(ctx context.Context, generation uint64) error
}

type AsymmetricKeyPairRepository interface {
	CreatePrivateKey(ctx context.Context, filePath string, privateKey *rsa.PrivateKey) error
	CreatePublicKey(ctx context.Context, filePath string, publicKey *rsa.PublicKey) error
	ReadRSAPublicKey(ctx context.Context, filePath string) (*rsa.PublicKey, error)
	ReadRSAPrivateKey(ctx context.Context, filePath string) (*rsa.PrivateKey, error)
	PublicKeyFingerPrint(ctx context.Context, publicKey *rsa.PublicKey) (string, error)
}

type SymmetricKeyRepository interface {
	GenerateSymmetricKey(ctx context.Context) ([]byte, error)
	EncryptSymmetricKey(ctx context.Context, publicKey *rsa.PublicKey, symmetricKey []byte) ([]byte, error)
	DecodeAndDecryptSymmetricKey(ctx context.Context, privateKey *rsa.PrivateKey, encodedKey string) ([]byte, error)
}

type LockRepository interface {
	Save(ctx context.Context, lock *repository.Lock) error
	List(ctx context.Context) ([]repository.Lock, error)
	Delete(ctx context.Context, lockID uuid.UUID) error
}
