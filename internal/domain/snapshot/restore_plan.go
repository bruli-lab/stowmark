package snapshot

import (
	"slices"
	"strings"
)

type RestorePlan struct {
	directories []File
	files       []File
	symlinks    []File
}

func (r RestorePlan) Directories() []File {
	return r.directories
}

func (r RestorePlan) Files() []File {
	return r.files
}

func (r RestorePlan) Symlinks() []File {
	return r.symlinks
}

func (r RestorePlan) DirectoriesDeepestFirst() []File {
	deepest := slices.Clone(r.directories)
	slices.Reverse(deepest)
	return deepest
}

func NewRestorePlan(entries []File) RestorePlan {
	plan := RestorePlan{}
	for _, entry := range entries {
		switch entry.Kind() {
		case DirectoryEntry:
			plan.directories = append(plan.directories, entry)
		case SymlinkEntry:
			plan.symlinks = append(plan.symlinks, entry)
		case RegularEntry:
			plan.files = append(plan.files, entry)
		}
	}
	slices.SortFunc(plan.directories, func(a, b File) int {
		return strings.Compare(a.Path(), b.Path())
	})
	return plan
}
