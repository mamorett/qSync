package planner

import (
	"testing"
	"time"

	"github.com/mamorett/PhotoLib/internal/snapshot"
)

func mf(entries ...snapshot.Entry) *snapshot.Manifest {
	m := snapshot.NewManifest("h", "/r", time.Time{})
	for _, e := range entries {
		m.Entries[e.Path] = e
	}
	return m
}

func file(path string, size, mtime int64) snapshot.Entry {
	return snapshot.Entry{Path: path, Size: size, ModTime: mtime, Type: snapshot.TypeFile}
}

func findChange(p *Plan, path string) *Change {
	for i := range p.Changes {
		if p.Changes[i].Path == path {
			return &p.Changes[i]
		}
	}
	return nil
}

func TestBuild_PullAdd(t *testing.T) {
	synced := mf()
	local := mf()
	remote := mf(file("a", 10, 100))
	p := Build(DirectionPull, synced, local, remote, nil)
	c := findChange(p, "a")
	if c == nil || c.Kind != ChangeAdd || c.Reason != ReasonNewFile {
		t.Fatalf("expected add new file, got %+v", c)
	}
	if p.Stats.Additions != 1 || p.Stats.BytesTotal != 10 {
		t.Fatalf("stats wrong: %+v", p.Stats)
	}
}

func TestBuild_PullUpdateSize(t *testing.T) {
	synced := mf(file("a", 10, 100))
	local := mf(file("a", 10, 100))
	remote := mf(file("a", 20, 100))
	p := Build(DirectionPull, synced, local, remote, nil)
	c := findChange(p, "a")
	if c == nil || c.Kind != ChangeUpdate || c.Reason != ReasonSize {
		t.Fatalf("expected size-differs update, got %+v", c)
	}
}

func TestBuild_PullUpdateMtime(t *testing.T) {
	local := mf(file("a", 10, 100))
	remote := mf(file("a", 10, 200))
	p := Build(DirectionPull, mf(), local, remote, nil)
	c := findChange(p, "a")
	if c == nil || c.Reason != ReasonMtime {
		t.Fatalf("expected mtime-differs, got %+v", c)
	}
}

func TestBuild_PullDeleteStaged(t *testing.T) {
	local := mf(file("a", 10, 100))
	remote := mf()
	p := Build(DirectionPull, mf(file("a", 10, 100)), local, remote, nil)
	c := findChange(p, "a")
	if c == nil || c.Kind != ChangeDelete {
		t.Fatalf("expected delete, got %+v", c)
	}
	if p.Stats.Deletions != 1 {
		t.Fatalf("deletions stat = %d", p.Stats.Deletions)
	}
}

func TestBuild_TypeChanged(t *testing.T) {
	local := mf(snapshot.Entry{Path: "a", Type: snapshot.TypeFile, Size: 10})
	remote := mf(snapshot.Entry{Path: "a", Type: snapshot.TypeDir})
	p := Build(DirectionPull, mf(), local, remote, nil)
	c := findChange(p, "a")
	if c == nil || c.Reason != ReasonType {
		t.Fatalf("expected type changed, got %+v", c)
	}
}

func TestBuild_SortedDeterministic(t *testing.T) {
	remote := mf(file("z", 1, 1), file("a", 1, 1), file("m", 1, 1))
	p := Build(DirectionPull, mf(), mf(), remote, nil)
	prev := ""
	for _, c := range p.Changes {
		if c.Path < prev {
			t.Fatalf("changes not sorted: %s after %s", c.Path, prev)
		}
		prev = c.Path
	}
}

func TestBuild_ConflictExcludedFromChanges(t *testing.T) {
	local := mf(file("a", 2, 200))
	remote := mf(file("a", 3, 300))
	conflicts := []Conflict{{Path: "a", Detail: "both sides modified"}}
	p := Build(DirectionPull, mf(file("a", 1, 100)), local, remote, conflicts)
	if findChange(p, "a") != nil {
		t.Fatal("conflicted path should not appear as a clean change")
	}
	if p.Stats.Conflicts != 1 {
		t.Fatalf("conflict stat = %d", p.Stats.Conflicts)
	}
}
