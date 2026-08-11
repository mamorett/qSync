package conflict

import (
	"strings"
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

func TestDetect_NoChange(t *testing.T) {
	s := mf(file("a", 1, 100))
	l := mf(file("a", 1, 100))
	r := mf(file("a", 1, 100))
	if c := Detect(s, l, r); len(c) != 0 {
		t.Fatalf("expected no conflicts, got %v", c)
	}
}

func TestDetect_BothModified(t *testing.T) {
	s := mf(file("a", 1, 100))
	l := mf(file("a", 2, 200)) // local changed
	r := mf(file("a", 3, 300)) // remote changed differently
	c := Detect(s, l, r)
	if len(c) != 1 || !strings.HasPrefix(c[0].Detail, "both sides modified") {
		t.Fatalf("expected both-modified conflict, got %v", c)
	}
}

func TestDetect_Convergent(t *testing.T) {
	s := mf(file("a", 1, 100))
	l := mf(file("a", 2, 200))
	r := mf(file("a", 2, 200)) // identical to local
	if c := Detect(s, l, r); len(c) != 0 {
		t.Fatalf("convergent change should be no-op, got %v", c)
	}
}

func TestDetect_LocalEditRemoteDelete(t *testing.T) {
	s := mf(file("a", 1, 100))
	l := mf(file("a", 2, 200)) // edited locally
	r := mf()                  // deleted remotely
	c := Detect(s, l, r)
	if len(c) != 1 || c[0].Detail != "edited locally, deleted remotely" {
		t.Fatalf("got %v", c)
	}
}

func TestDetect_RemoteEditLocalDelete(t *testing.T) {
	s := mf(file("a", 1, 100))
	l := mf()                  // deleted locally
	r := mf(file("a", 2, 200)) // edited remotely
	c := Detect(s, l, r)
	if len(c) != 1 || c[0].Detail != "deleted locally, edited remotely" {
		t.Fatalf("got %v", c)
	}
}

func TestDetect_DeletedBoth(t *testing.T) {
	s := mf(file("a", 1, 100))
	l := mf()
	r := mf()
	if c := Detect(s, l, r); len(c) != 0 {
		t.Fatalf("deleted-both should converge, got %v", c)
	}
}

func TestDetect_OneSidedCleanChange(t *testing.T) {
	s := mf(file("a", 1, 100))
	l := mf(file("a", 1, 100)) // unchanged
	r := mf(file("a", 2, 200)) // remote changed only
	if c := Detect(s, l, r); len(c) != 0 {
		t.Fatalf("one-sided change is clean, got %v", c)
	}
}

func TestDetect_FirstRunEmptyAncestor(t *testing.T) {
	// No synced ancestor: both sides have differing content => conflict.
	l := mf(file("a", 1, 100))
	r := mf(file("a", 2, 200))
	c := Detect(nil, l, r)
	if len(c) != 1 {
		t.Fatalf("first-run differing file should conflict, got %v", c)
	}
}

func TestDetect_FirstRunOneSidedIsClean(t *testing.T) {
	l := mf(file("a", 1, 100))
	r := mf() // only local
	if c := Detect(nil, l, r); len(c) != 0 {
		t.Fatalf("first-run one-sided path is a clean add, got %v", c)
	}
}

func TestDetect_VFATTimestampTolerance(t *testing.T) {
	l := mf(file("a", 100, 1000))
	r := mf(file("a", 100, 1002)) // 2 seconds diff due to VFAT rounding

	// Standard detect: conflict because 1000 != 1002
	if c := Detect(nil, l, r); len(c) != 1 {
		t.Fatalf("expected 1 conflict without VFAT mode, got %v", c)
	}

	// VFAT detect: tolerance <= 2 seconds -> no conflict
	if c := DetectVFAT(nil, l, r, true); len(c) != 0 {
		t.Fatalf("expected 0 conflicts with VFAT mode, got %v", c)
	}
}
