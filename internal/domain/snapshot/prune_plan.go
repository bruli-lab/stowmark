package snapshot

import "slices"

type PrunePlan struct {
	stored            int
	referenced        int
	unreferenced      []string
	unreferencedBytes int64
	unreferencedSizes map[string]int64
	missing           []string
	generations       []uint64
}

func NewPrunePlan(manifests []Manifest, stored []StoredObject) *PrunePlan {
	referenced := map[string]struct{}{}
	for i := range manifests {
		for _, hash := range manifests[i].ReferencedHashes() {
			referenced[hash] = struct{}{}
		}
	}

	present := make(map[string]struct{}, len(stored))
	unreferenced := make([]string, 0)
	unreferencedSizes := make(map[string]int64)
	var unreferencedBytes int64
	for _, object := range stored {
		if _, seen := present[object.hash]; seen {
			continue
		}
		present[object.hash] = struct{}{}
		if _, ok := referenced[object.hash]; !ok {
			unreferenced = append(unreferenced, object.hash)
			unreferencedBytes += object.size
			unreferencedSizes[object.hash] = object.size
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
		stored:            len(present),
		referenced:        len(referenced),
		unreferenced:      unreferenced,
		unreferencedBytes: unreferencedBytes,
		unreferencedSizes: unreferencedSizes,
		missing:           missing,
	}
}

func (p *PrunePlan) UnreferencedBytes() int64 {
	return p.unreferencedBytes
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

func (p *PrunePlan) RecordOrphanGenerations(generations []uint64) {
	p.generations = generations
}

func (p *PrunePlan) OrphanGenerations() []uint64 {
	return p.generations
}
