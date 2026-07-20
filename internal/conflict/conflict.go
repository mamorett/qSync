package conflict

import (
	"sort"

	"github.com/yourorg/qsync/internal/planner"
	"github.com/yourorg/qsync/internal/snapshot"
)

// entryDiffers reports whether two entries differ in existence/size/mtime/type.
// Mode is ignored (mode-only changes are updates, not conflicts). Hash is
// ignored (only verify --checksum uses hashes).
func entryDiffers(a, aOK bool, ae snapshot.Entry, b, bOK bool, be snapshot.Entry) bool {
	if aOK != bOK {
		return true
	}
	if !aOK {
		return false
	}
	if ae.Type != be.Type {
		return true
	}
	if ae.Type == snapshot.TypeSymlink {
		return ae.LinkTarget != be.LinkTarget
	}
	if ae.Type == snapshot.TypeDir {
		return false
	}
	return ae.Size != be.Size || ae.ModTime != be.ModTime
}

// Detect performs three-way conflict detection between the synced ancestor,
// local, and remote manifests. It returns conflicts sorted by path.
//
// First-run special case: if synced is nil or empty, the ancestor is treated as
// empty, so any path existing on both sides with differing content is a
// conflict.
func Detect(synced, local, remote *snapshot.Manifest) []planner.Conflict {
	syncedEntries := map[string]snapshot.Entry{}
	if synced != nil {
		syncedEntries = synced.Entries
	}

	// Collect the union of all paths.
	paths := map[string]struct{}{}
	for p := range syncedEntries {
		paths[p] = struct{}{}
	}
	for p := range local.Entries {
		paths[p] = struct{}{}
	}
	for p := range remote.Entries {
		paths[p] = struct{}{}
	}

	var conflicts []planner.Conflict
	for p := range paths {
		se, sOK := syncedEntries[p]
		le, lOK := local.Entries[p]
		re, rOK := remote.Entries[p]

		localChanged := entryDiffers(sOK, sOK, se, lOK, lOK, le)
		remoteChanged := entryDiffers(sOK, sOK, se, rOK, rOK, re)

		if !localChanged && !remoteChanged {
			continue
		}

		// Rule 4: deleted on both => converged.
		if !lOK && !rOK {
			continue
		}

		// Rule 2: local changed, remote deleted (ancestor existed).
		if localChanged && lOK && !rOK && sOK {
			conflicts = append(conflicts, mkConflict(p, le, re, lOK, rOK, "edited locally, deleted remotely"))
			continue
		}

		// Rule 3: remote changed, local deleted (ancestor existed).
		if remoteChanged && rOK && !lOK && sOK {
			conflicts = append(conflicts, mkConflict(p, le, re, lOK, rOK, "deleted locally, edited remotely"))
			continue
		}

		// Rule 1: both changed and both still present.
		if localChanged && remoteChanged && lOK && rOK {
			// Convergent change: identical result on both sides => no-op.
			if convergent(le, re) {
				continue
			}
			conflicts = append(conflicts, mkConflict(p, le, re, lOK, rOK, "both sides modified"))
			continue
		}

		// Otherwise: only one side changed cleanly (handled by planner).
	}

	sort.Slice(conflicts, func(i, j int) bool { return conflicts[i].Path < conflicts[j].Path })
	return conflicts
}

// convergent reports whether two entries represent the same content
// (same size and mtime, same type). A file-vs-directory collision is never
// convergent.
func convergent(a, b snapshot.Entry) bool {
	if a.Type != b.Type {
		return false
	}
	if a.Type == snapshot.TypeSymlink {
		return a.LinkTarget == b.LinkTarget
	}
	if a.Type == snapshot.TypeDir {
		return true
	}
	return a.Size == b.Size && a.ModTime == b.ModTime
}

func mkConflict(path string, le, re snapshot.Entry, lOK, rOK bool, detail string) planner.Conflict {
	c := planner.Conflict{Path: path, Detail: detail}
	if lOK {
		c.LocalMtime = le.ModTime
	}
	if rOK {
		c.RemoteMtime = re.ModTime
	}
	return c
}
