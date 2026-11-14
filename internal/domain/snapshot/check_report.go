package snapshot

import (
	"maps"
	"slices"
	"strings"
)

type ManifestProblem struct {
	snapshotID string
	reason     string
}

func (m ManifestProblem) SnapshotID() string {
	return m.snapshotID
}

func (m ManifestProblem) Reason() string {
	return m.reason
}

func NewManifestProblem(snapshotID, reason string) ManifestProblem {
	return ManifestProblem{snapshotID: snapshotID, reason: reason}
}

type ObjectProblem struct {
	hash   string
	reason string
}

func (o ObjectProblem) Hash() string {
	return o.hash
}

func (o ObjectProblem) Reason() string {
	return o.reason
}

func NewObjectProblem(hash, reason string) ObjectProblem {
	return ObjectProblem{hash: hash, reason: reason}
}

type MissingObject struct {
	hash      string
	snapshots []string
}

func (m MissingObject) Hash() string {
	return m.hash
}

func (m MissingObject) Snapshots() []string {
	return m.snapshots
}

type CheckReport struct {
	snapshots         int
	corruptManifests  []ManifestProblem
	storedObjects     int
	referencedObjects int
	unreferenced      int
	missing           []MissingObject
	readable          []string
	sizes             map[string]int64
	staleLocks        int
	orphanGenerations []uint64
	readData          bool
	readObjects       int
	corruptObjects    []ObjectProblem
}

func NewCheckReport(manifests []Manifest, corruptManifests []ManifestProblem, stored []string) *CheckReport {
	plan := NewPrunePlan(manifests, stored)

	missingSnapshots := make(map[string][]string, len(plan.Missing()))
	for _, hash := range plan.Missing() {
		missingSnapshots[hash] = nil
	}
	for i := range manifests {
		for _, hash := range manifests[i].ReferencedHashes() {
			snapshots, ok := missingSnapshots[hash]
			if ok && !slices.Contains(snapshots, manifests[i].ID()) {
				missingSnapshots[hash] = append(snapshots, manifests[i].ID())
			}
		}
	}
	missing := make([]MissingObject, 0, len(plan.Missing()))
	for _, hash := range plan.Missing() {
		snapshots := missingSnapshots[hash]
		slices.Sort(snapshots)
		missing = append(missing, MissingObject{hash: hash, snapshots: snapshots})
	}

	sizes := make(map[string]int64)
	for i := range manifests {
		maps.Copy(sizes, manifests[i].ObjectSizes())
	}

	unreferenced := plan.Unreferenced()
	readable := make([]string, 0, plan.StoredObjects()-len(unreferenced))
	for _, hash := range slices.Compact(slices.Sorted(slices.Values(stored))) {
		if _, isUnreferenced := slices.BinarySearch(unreferenced, hash); !isUnreferenced {
			readable = append(readable, hash)
		}
	}

	return &CheckReport{
		snapshots:         len(manifests),
		corruptManifests:  corruptManifests,
		storedObjects:     plan.StoredObjects(),
		referencedObjects: plan.ReferencedObjects(),
		unreferenced:      len(unreferenced),
		missing:           missing,
		readable:          readable,
		sizes:             sizes,
	}
}

func (c *CheckReport) RecordStaleLocks(count int) {
	c.staleLocks = count
}

func (c *CheckReport) RecordOrphanGenerations(generations []uint64) {
	c.orphanGenerations = generations
}

func (c *CheckReport) RecordData(readObjects int, corrupt []ObjectProblem) {
	c.readData = true
	c.readObjects = readObjects
	c.corruptObjects = slices.SortedFunc(slices.Values(corrupt), func(a, b ObjectProblem) int {
		return strings.Compare(a.hash, b.hash)
	})
}

func (c *CheckReport) ReadableObjects() []string {
	return c.readable
}

func (c *CheckReport) ObjectSize(hash string) int64 {
	return c.sizes[hash]
}

func (c *CheckReport) Snapshots() int {
	return c.snapshots
}

func (c *CheckReport) CorruptManifests() []ManifestProblem {
	return c.corruptManifests
}

func (c *CheckReport) StoredObjects() int {
	return c.storedObjects
}

func (c *CheckReport) ReferencedObjects() int {
	return c.referencedObjects
}

func (c *CheckReport) UnreferencedObjects() int {
	return c.unreferenced
}

func (c *CheckReport) MissingObjects() []MissingObject {
	return c.missing
}

func (c *CheckReport) StaleLocks() int {
	return c.staleLocks
}

func (c *CheckReport) OrphanGenerations() []uint64 {
	return c.orphanGenerations
}

func (c *CheckReport) ReadData() bool {
	return c.readData
}

func (c *CheckReport) ReadObjects() int {
	return c.readObjects
}

func (c *CheckReport) CorruptObjects() []ObjectProblem {
	return c.corruptObjects
}

func (c *CheckReport) HasErrors() bool {
	return len(c.corruptManifests) > 0 || len(c.missing) > 0 || len(c.corruptObjects) > 0
}
