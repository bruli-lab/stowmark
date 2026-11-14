package snapshot

import "slices"

type PrunePlan struct {
	stored       int
	referenced   int
	unreferenced []string
	missing      []string
}

func NewPrunePlan(manifests []Manifest, stored []string) *PrunePlan {
	referenced := map[string]struct{}{}
	for i := range manifests {
		for _, hash := range manifests[i].ReferencedHashes() {
			referenced[hash] = struct{}{}
		}
	}

	present := make(map[string]struct{}, len(stored))
	unreferenced := make([]string, 0)
	for _, hash := range stored {
		if _, seen := present[hash]; seen {
			continue
		}
		present[hash] = struct{}{}
		if _, ok := referenced[hash]; !ok {
			unreferenced = append(unreferenced, hash)
		}
	}

	missing := make([]string, 0)
	for hash := range referenced {
		if _, ok := present[hash]; !ok {
			missing = append(missing, hash)
		}
	}

	slices.Sort(unreferenced)
	slices.Sort(missing)

	return &PrunePlan{
		stored:       len(present),
		referenced:   len(referenced),
		unreferenced: unreferenced,
		missing:      missing,
	}
}

func (p *PrunePlan) StoredObjects() int {
	return p.stored
}

func (p *PrunePlan) ReferencedObjects() int {
	return p.referenced
}

func (p *PrunePlan) Unreferenced() []string {
	return p.unreferenced
}

func (p *PrunePlan) Missing() []string {
	return p.missing
}
