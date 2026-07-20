package planner

import (
	"sort"

	"github.com/yourorg/photolib/internal/snapshot"
)

// Direction is the direction of a sync operation.
type Direction int

const (
	// DirectionPull moves changes from remote (source of truth) to local.
	DirectionPull Direction = iota
	// DirectionPush moves changes from local to remote.
	DirectionPush
)

func (d Direction) String() string {
	if d == DirectionPush {
		return "push"
	}
	return "pull"
}

// ChangeKind classifies a planned change.
type ChangeKind int

const (
	// ChangeAdd: exists only on the source side.
	ChangeAdd ChangeKind = iota
	// ChangeUpdate: exists both sides, content differs.
	ChangeUpdate
	// ChangeDelete: exists only on the destination (staged, never auto-applied).
	ChangeDelete
)

func (k ChangeKind) String() string {
	switch k {
	case ChangeAdd:
		return "add"
	case ChangeUpdate:
		return "update"
	case ChangeDelete:
		return "delete"
	default:
		return "unknown"
	}
}

// Change is a single planned modification.
type Change struct {
	Path    string     `json:"path"`
	Kind    ChangeKind `json:"kind"`
	Size    int64      `json:"size,omitempty"`
	OldSize int64      `json:"old_size,omitempty"`
	Reason  string     `json:"reason"`
}

// Conflict is a detected three-way conflict.
type Conflict struct {
	Path        string `json:"path"`
	LocalMtime  int64  `json:"local_mtime"`
	RemoteMtime int64  `json:"remote_mtime"`
	Detail      string `json:"detail"`
}

// PlanStats summarizes a plan.
type PlanStats struct {
	Additions  int   `json:"additions"`
	Updates    int   `json:"updates"`
	Deletions  int   `json:"deletions"`
	Conflicts  int   `json:"conflicts"`
	BytesTotal int64 `json:"bytes_total"`
}

// Plan is the full computed set of changes for one direction.
type Plan struct {
	Direction Direction  `json:"direction"`
	Source    string     `json:"source"`
	Dest      string     `json:"dest"`
	Changes   []Change   `json:"changes"`
	Conflicts []Conflict `json:"conflicts"`
	Stats     PlanStats  `json:"stats"`
}

// Reason strings (tests assert on these exact values).
const (
	ReasonNewFile   = "new file"
	ReasonSize      = "size differs"
	ReasonMtime     = "mtime differs"
	ReasonChecksum  = "checksum differs"
	ReasonType      = "type changed"
	reasonDeleteFmt = "deleted on "
)

// Build produces a Plan for the given direction from the three manifests.
// Conflicts are detected first; even when present, the full plan is returned so
// callers can display both. The executor is responsible for refusing to apply a
// plan that has conflicts.
//
// ConflictFn is injected to avoid an import cycle with the conflict package.
func Build(direction Direction, synced, local, remote *snapshot.Manifest, conflicts []Conflict) *Plan {
	var srcM, dstM *snapshot.Manifest
	switch direction {
	case DirectionPull:
		srcM, dstM = remote, local
	default: // DirectionPush
		srcM, dstM = local, remote
	}

	// Paths that are conflicted must be excluded from clean changes.
	conflicted := make(map[string]bool, len(conflicts))
	for _, c := range conflicts {
		conflicted[c.Path] = true
	}

	p := &Plan{
		Direction: direction,
		Changes:   []Change{},
		Conflicts: append([]Conflict{}, conflicts...),
	}

	sideName := "remote"
	if direction == DirectionPush {
		sideName = "local"
	}

	// Adds and updates: paths where source differs from dest.
	for _, path := range srcM.SortedPaths() {
		if conflicted[path] {
			continue
		}
		se := srcM.Entries[path]
		de, ok := dstM.Entries[path]
		if !ok {
			p.Changes = append(p.Changes, Change{
				Path:   path,
				Kind:   ChangeAdd,
				Size:   se.Size,
				Reason: ReasonNewFile,
			})
			continue
		}
		reason, differs := diffReason(de, se)
		if differs {
			p.Changes = append(p.Changes, Change{
				Path:    path,
				Kind:    ChangeUpdate,
				Size:    se.Size,
				OldSize: de.Size,
				Reason:  reason,
			})
		}
	}

	// Deletions: paths in dest but not in source (staged, never sent to rsync).
	for _, path := range dstM.SortedPaths() {
		if conflicted[path] {
			continue
		}
		if _, ok := srcM.Entries[path]; !ok {
			de := dstM.Entries[path]
			p.Changes = append(p.Changes, Change{
				Path:   path,
				Kind:   ChangeDelete,
				Size:   de.Size,
				Reason: reasonDeleteFmt + oppositeSide(sideName),
			})
		}
	}

	sort.Slice(p.Changes, func(i, j int) bool { return p.Changes[i].Path < p.Changes[j].Path })
	sort.Slice(p.Conflicts, func(i, j int) bool { return p.Conflicts[i].Path < p.Conflicts[j].Path })

	for _, c := range p.Changes {
		switch c.Kind {
		case ChangeAdd:
			p.Stats.Additions++
			p.Stats.BytesTotal += c.Size
		case ChangeUpdate:
			p.Stats.Updates++
			p.Stats.BytesTotal += c.Size
		case ChangeDelete:
			p.Stats.Deletions++
		}
	}
	p.Stats.Conflicts = len(p.Conflicts)
	return p
}

// oppositeSide returns which side a deletion originated from, for the reason text.
func oppositeSide(src string) string {
	if src == "remote" {
		// pull: file exists locally but not remotely => deleted on remote
		return "remote"
	}
	return "local"
}

// diffReason returns the reason a destination entry differs from a source entry.
func diffReason(dst, src snapshot.Entry) (string, bool) {
	if dst.Type != src.Type {
		return ReasonType, true
	}
	if src.Type == snapshot.TypeSymlink {
		if dst.LinkTarget != src.LinkTarget {
			return ReasonType, true
		}
		return "", false
	}
	if src.Type == snapshot.TypeDir {
		return "", false
	}
	if dst.Size != src.Size {
		return ReasonSize, true
	}
	if dst.ModTime != src.ModTime {
		return ReasonMtime, true
	}
	return "", false
}
