package snapshot

import "slices"

type References struct {
	hashes map[string]struct{}
}

func NewReferences(manifests []Manifest) *References {
	references := &References{hashes: map[string]struct{}{}}
	for i := range manifests {
		for _, hash := range manifests[i].ReferencedHashes() {
			references.hashes[hash] = struct{}{}
		}
	}
	return references
}

func (r *References) Len() int {
	return len(r.hashes)
}

func (r *References) Contains(hash string) bool {
	_, ok := r.hashes[hash]
	return ok
}

func (r *References) Unreferenced(stored []string) []string {
	unreferenced := make([]string, 0)
	for _, hash := range stored {
		if !r.Contains(hash) {
			unreferenced = append(unreferenced, hash)
		}
	}
	slices.Sort(unreferenced)
	return slices.Compact(unreferenced)
}

func (r *References) Missing(stored []string) []string {
	present := make(map[string]struct{}, len(stored))
	for _, hash := range stored {
		present[hash] = struct{}{}
	}
	missing := make([]string, 0)
	for hash := range r.hashes {
		if _, ok := present[hash]; !ok {
			missing = append(missing, hash)
		}
	}
	slices.Sort(missing)
	return missing
}
