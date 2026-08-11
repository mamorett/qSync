package conflict

import (
	"fmt"
	"sort"
	"time"

	"github.com/mamorett/qsync/internal/planner"
	"github.com/mamorett/qsync/internal/snapshot"
)

func mtimeEqual(t1, t2 int64, vfat bool) bool {
	if t1 == t2 {
		return true
	}
	diff := t1 - t2
	if diff < 0 {
		diff = -diff
	}
	if vfat {
		// 2-second VFAT resolution window
		if diff <= 2 {
			return true
		}
		// FAT32/exFAT local-time vs UTC timezone offsets (1h=3600s, 2h=7200s ± 2s)
		if (diff >= 3598 && diff <= 3602) || (diff >= 7198 && diff <= 7202) {
			return true
		}
	}
	return false
}

// entryDiffers reports whether two entries differ in existence/size/mtime/type.
// Mode is ignored (mode-only changes are updates, not conflicts). Hash is
// ignored (only verify --checksum uses hashes).
func entryDiffers(a, aOK bool, ae snapshot.Entry, b, bOK bool, be snapshot.Entry, vfat bool) bool {
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
	return ae.Size != be.Size || !mtimeEqual(ae.ModTime, be.ModTime, vfat)
}

// Detect performs three-way conflict detection between the synced ancestor,
// local, and remote manifests. It returns conflicts sorted by path.
func Detect(synced, local, remote *snapshot.Manifest) []planner.Conflict {
	return DetectVFAT(synced, local, remote, false)
}

// DetectVFAT performs three-way conflict detection with optional VFAT 2-second timestamp tolerance.
func DetectVFAT(synced, local, remote *snapshot.Manifest, vfat bool) []planner.Conflict {
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

		localChanged := entryDiffers(sOK, sOK, se, lOK, lOK, le, vfat)
		remoteChanged := entryDiffers(sOK, sOK, se, rOK, rOK, re, vfat)

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
			if convergent(le, re, vfat) {
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
func convergent(a, b snapshot.Entry, vfat bool) bool {
	if a.Type != b.Type {
		return false
	}
	if a.Type == snapshot.TypeSymlink {
		return a.LinkTarget == b.LinkTarget
	}
	if a.Type == snapshot.TypeDir {
		return true
	}
	return a.Size == b.Size && mtimeEqual(a.ModTime, b.ModTime, vfat)
}

func mkConflict(path string, le, re snapshot.Entry, lOK, rOK bool, baseDetail string) planner.Conflict {
	detail := baseDetail
	if lOK && rOK {
		if le.Type != re.Type {
			detail = fmt.Sprintf("%s (type: local=%v, remote=%v)", baseDetail, le.Type, re.Type)
		} else if le.Size != re.Size {
			detail = fmt.Sprintf("%s (size: local=%d, remote=%d)", baseDetail, le.Size, re.Size)
		} else if le.ModTime != re.ModTime {
			diff := le.ModTime - re.ModTime
			if diff < 0 {
				diff = -diff
			}
			lStr := time.Unix(le.ModTime, 0).UTC().Format("15:04:05")
			rStr := time.Unix(re.ModTime, 0).UTC().Format("15:04:05")
			detail = fmt.Sprintf("%s (mtime diff %ds: local=%s, remote=%s)", baseDetail, diff, lStr, rStr)
		}
	}
	c := planner.Conflict{Path: path, Detail: detail}
	if lOK {
		c.LocalMtime = le.ModTime
	}
	if rOK {
		c.RemoteMtime = re.ModTime
	}
	return c
}
