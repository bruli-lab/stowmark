package blob

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/domain/snapshot"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/model"
	"golang.org/x/sync/errgroup"
)

const manifestReadWorkers = 8

type ManifestRepository struct {
	store Store
}

var (
	ErrEncryptedManifest   = errors.New("the manifest is encrypted: the private key is required")
	ErrUnencryptedManifest = errors.New("the manifest of an encrypted repository is not encrypted")
)

func (m ManifestRepository) Save(ctx context.Context, manifest *snapshot.Manifest, codec repository.Codec) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key, err := manifestKey(manifest.ID())
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(model.NewManifest(manifest), "", " ")
	if err != nil {
		return fmt.Errorf("marshal manifest %q: %w", manifest.ID(), err)
	}
	data = append(data, '\n')

	return m.store.Put(ctx, key, func(destination io.Writer) error {
		if !codec.EncryptsMetadata() {
			_, err := destination.Write(data)
			return err
		}
		return encryptManifest(destination, data, manifest.ID(), codec)
	})
}

func encryptManifest(destination io.Writer, data []byte, snapshotID string, codec repository.Codec) error {
	key, err := encrypt.ManifestKey(codec.MetadataKey())
	if err != nil {
		return err
	}
	stream, err := encrypt.NewStreamWriter(destination, key, manifestAAD(snapshotID))
	if err != nil {
		return fmt.Errorf("encrypt manifest %q: %w", snapshotID, err)
	}
	if _, err := stream.Write(data); err != nil {
		return fmt.Errorf("encrypt manifest %q: %w", snapshotID, err)
	}
	return stream.Close()
}

func decryptManifest(data []byte, snapshotID string, codec repository.Codec) ([]byte, error) {
	if !codec.EncryptsMetadata() {
		if encrypt.IsStream(data) {
			return nil, ErrEncryptedManifest
		}
		return data, nil
	}
	if !encrypt.IsStream(data) {
		return nil, ErrUnencryptedManifest
	}
	key, err := encrypt.ManifestKey(codec.MetadataKey())
	if err != nil {
		return nil, err
	}
	stream, err := encrypt.NewStreamReader(bytes.NewReader(data), key, manifestAAD(snapshotID))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(stream)
}

func manifestAAD(snapshotID string) []byte {
	return []byte("manifest/" + snapshotID)
}

func (m ManifestRepository) Get(ctx context.Context, snapshotID string, codec repository.Codec) (*snapshot.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key, err := manifestKey(snapshotID)
	if err != nil {
		return nil, err
	}

	return m.read(ctx, key, codec)
}

func (m ManifestRepository) List(ctx context.Context, codec repository.Codec) ([]snapshot.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := m.store.List(ctx, repository.SnapshotsFolder)
	if err != nil {
		return nil, fmt.Errorf("list manifests: %w", err)
	}
	keys := Keys(entries)

	keys = slices.DeleteFunc(keys, func(key string) bool { return !isManifestKey(key) })
	manifests := make([]snapshot.Manifest, len(keys))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(manifestReadWorkers)
	for i, key := range keys {
		if groupCtx.Err() != nil {
			break
		}
		group.Go(func() error {
			manifest, err := m.read(groupCtx, key, codec)
			if err != nil {
				return err
			}
			manifests[i] = *manifest
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	slices.SortFunc(manifests, func(a, b snapshot.Manifest) int {
		return b.CreatedAt().Compare(a.CreatedAt())
	})

	return manifests, nil
}

func (m ManifestRepository) ListIDs(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := m.store.List(ctx, repository.SnapshotsFolder)
	if err != nil {
		return nil, fmt.Errorf("list manifests: %w", err)
	}
	keys := Keys(entries)

	ids := make([]string, 0, len(keys))
	for _, key := range keys {
		if isManifestKey(key) {
			ids = append(ids, manifestID(key))
		}
	}
	slices.Sort(ids)

	return ids, nil
}

func (m ManifestRepository) Delete(ctx context.Context, snapshotID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	key, err := manifestKey(snapshotID)
	if err != nil {
		return err
	}

	if err := m.store.Delete(ctx, key); err != nil {
		return fmt.Errorf("delete manifest %q: %w", snapshotID, err)
	}

	return nil
}

func (m ManifestRepository) read(ctx context.Context, key string, codec repository.Codec) (*snapshot.Manifest, error) {
	reader, err := m.store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, repository.NewNotFoundError("manifest not found")
		}
		return nil, fmt.Errorf("open manifest %q: %w", key, err)
	}
	defer func() {
		_ = reader.Close()
	}()

	data, err := io.ReadAll(model.ContextReader{Ctx: ctx, Reader: reader})
	if err != nil {
		return nil, fmt.Errorf("read manifest %q: %w", key, err)
	}

	decrypted, err := decryptManifest(data, manifestID(key), codec)
	if err != nil {
		return nil, fmt.Errorf("read manifest %q: %w", key, err)
	}

	return model.BuildManifestDomain(decrypted, key)
}

func manifestID(key string) string {
	return strings.TrimSuffix(path.Base(key), manifestExtension)
}

func NewManifestRepository(store Store) *ManifestRepository {
	return &ManifestRepository{store: store}
}
