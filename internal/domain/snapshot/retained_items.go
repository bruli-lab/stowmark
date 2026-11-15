package snapshot

import (
	"slices"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
)

func RetainedItems(manifests []Manifest, conf *repository.Config) []repository.RetainedItem {
	hashes := map[string]struct{}{}
	for i := range manifests {
		for _, hash := range manifests[i].ReferencedHashes() {
			hashes[hash] = struct{}{}
		}
	}
	objects := make([]string, 0, len(hashes))
	for hash := range hashes {
		objects = append(objects, hash)
	}
	slices.Sort(objects)

	items := make([]repository.RetainedItem, 0, 1+len(manifests)+len(objects))
	items = append(items, repository.NewRetainedConfig())
	for i := range manifests {
		items = append(items, repository.NewRetainedManifest(manifests[i].ID()))
	}
	for _, hash := range objects {
		items = append(items, conf.RetainedObject(hash))
	}
	return items
}
