package app

import (
	"context"
	"crypto/rsa"
	"io"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/google/uuid"
)

//go:generate go tool moq -out repositories_mock.go . FolderRepository SourceRepository ManifestRepository ObjectRepository EncryptedObjectRepository RestoreDestination AsymmetricKeyPairRepository SymmetricKeyRepository LockRepository KeyGenerator
type FolderRepository interface {
	Exists(ctx context.Context, path string) (bool, error)
	CreateFolder(ctx context.Context, path string) error
	CreateConfig(ctx context.Context, path string, c *repository.Config) error
	GetConfig(ctx context.Context, path string) (*repository.Config, error)
}

type SourceRepository interface {
	IgnorePatterns(ctx context.Context, sourcePath string) ([]string, error)
	Explore(ctx context.Context, sourcePath string, rules *snapshot.ExcludeRules) (*snapshot.Source, error)
	CalculateHash(ctx context.Context, filePath string, codec repository.Codec) (string, error)
	CalculateChunks(ctx context.Context, filePath string, size int64, codec repository.Codec) ([]snapshot.Chunk, error)
}

type ManifestRepository interface {
	Save(ctx context.Context, m *snapshot.Manifest, codec repository.Codec) error
	List(ctx context.Context, codec repository.Codec) ([]snapshot.Manifest, error)
	ListIDs(ctx context.Context) ([]string, error)
	Get(ctx context.Context, snapshotID string, codec repository.Codec) (*snapshot.Manifest, error)
	Delete(ctx context.Context, snapshotID string) error
}

type ObjectRepository interface {
	Save(ctx context.Context, filePath, hash string, codec repository.Codec) error
	SaveChunk(ctx context.Context, filePath string, chunk snapshot.Chunk, codec repository.Codec) error
	AlreadyExists(ctx context.Context, hash string, codec repository.Codec) (bool, error)
	VerifyObject(ctx context.Context, hash string, codec repository.Codec) error
	RestoreObject(ctx context.Context, file *snapshot.File, codec repository.Codec) error
	ListObjects(ctx context.Context, encrypted bool, generation uint64) ([]snapshot.StoredObject, error)
	DeleteObject(ctx context.Context, hash string, encrypted bool, generation uint64) error
	DeleteTemporaryObjects(ctx context.Context) (int, error)
}

type EncryptedObjectRepository interface {
	ListEncryptedObjects(ctx context.Context, generation uint64) ([]string, error)
	ListEncryptedGenerations(ctx context.Context) ([]uint64, error)
	ReadEncryptedObject(ctx context.Context, hash string, codec repository.Codec) (io.ReadCloser, error)
	SaveRekeyedObject(ctx context.Context, hash string, source io.Reader, codec repository.Codec) error
	DeleteEncryptedGeneration(ctx context.Context, generation uint64) error
	AbortRekey(ctx context.Context, generation uint64) error
}

type RestoreDestination interface {
	CreateDirectory(ctx context.Context, path string) error
	CreateSymlink(ctx context.Context, path, target string) error
	ApplyMetadata(ctx context.Context, path string, metadata snapshot.Metadata) error
}

type AsymmetricKeyPairRepository interface {
	CreatePrivateKey(ctx context.Context, filePath string, privateKey *rsa.PrivateKey) error
	CreatePublicKey(ctx context.Context, filePath string, publicKey *rsa.PublicKey) error
	ReadRSAPublicKey(ctx context.Context, filePath string) (*rsa.PublicKey, error)
	ReadRSAPrivateKey(ctx context.Context, filePath string) (*rsa.PrivateKey, error)
	PublicKeyFingerPrint(ctx context.Context, publicKey *rsa.PublicKey) (string, error)
}

type KeyGenerator interface {
	GenerateRSAKey(ctx context.Context) (*rsa.PrivateKey, error)
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
