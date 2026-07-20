package verify

import (
	"testing"
	"time"

	"github.com/yourorg/photolib/internal/snapshot"
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

func TestVerify_Clean(t *testing.T) {
	l := mf(file("a", 1, 100), file("b", 2, 200))
	r := mf(file("a", 1, 100), file("b", 2, 200))
	res, err := Verify(l, r, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() || res.Checked != 2 {
		t.Fatalf("expected clean, got %+v", res)
	}
}

func TestVerify_SizeMismatch(t *testing.T) {
	l := mf(file("a", 1, 100))
	r := mf(file("a", 2, 100))
	res, _ := Verify(l, r, false, "")
	if res.OK() || res.Mismatches[0].Kind != "size" {
		t.Fatalf("expected size mismatch, got %+v", res.Mismatches)
	}
}

func TestVerify_MtimeMismatch(t *testing.T) {
	l := mf(file("a", 1, 100))
	r := mf(file("a", 1, 200))
	res, _ := Verify(l, r, false, "")
	if res.OK() || res.Mismatches[0].Kind != "mtime" {
		t.Fatalf("expected mtime mismatch, got %+v", res.Mismatches)
	}
}

func TestVerify_Missing(t *testing.T) {
	l := mf(file("a", 1, 100))
	r := mf(file("b", 1, 100))
	res, _ := Verify(l, r, false, "")
	if len(res.Mismatches) != 2 {
		t.Fatalf("expected 2 missing mismatches, got %+v", res.Mismatches)
	}
	kinds := map[string]bool{}
	for _, m := range res.Mismatches {
		kinds[m.Kind] = true
	}
	if !kinds["missing-local"] || !kinds["missing-remote"] {
		t.Errorf("missing kinds: %v", kinds)
	}
}
