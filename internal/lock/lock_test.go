package lock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireRelease(t *testing.T) {
	dir := t.TempDir()
	lk, err := Acquire(dir, "pull")
	if err != nil {
		t.Fatal(err)
	}
	// Lock file should exist with metadata.
	data, _ := os.ReadFile(LockPath(dir))
	if len(data) == 0 {
		t.Fatal("lock file empty after acquire")
	}
	if err := lk.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestLockContention(t *testing.T) {
	dir := t.TempDir()
	lk1, err := Acquire(dir, "pull")
	if err != nil {
		t.Fatal(err)
	}
	defer lk1.Release()

	_, err = Acquire(dir, "push")
	if err == nil {
		t.Fatal("second acquire should fail")
	}
	if !IsErrLocked(err) {
		t.Fatalf("expected ErrLocked, got %v", err)
	}
}

func TestIsLocked(t *testing.T) {
	dir := t.TempDir()
	// Not locked initially (no file).
	held, _, err := IsLocked(dir)
	if err != nil {
		t.Fatal(err)
	}
	if held {
		t.Fatal("should not be locked initially")
	}

	lk, _ := Acquire(dir, "pull")
	held, info, err := IsLocked(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !held {
		t.Fatal("should be locked while held")
	}
	if info.Operation != "pull" {
		t.Errorf("operation = %q", info.Operation)
	}
	lk.Release()

	held, _, _ = IsLocked(dir)
	if held {
		t.Fatal("should be free after release")
	}
}

func TestReleaseAfterTakeoverStale(t *testing.T) {
	dir := t.TempDir()
	// Write a lock file with a dead PID but do not hold flock.
	os.MkdirAll(filepath.Join(dir, ".photolib"), 0755)
	os.WriteFile(LockPath(dir), []byte(`{"pid":999999,"operation":"pull"}`), 0644)
	// Should be able to acquire (stale takeover or fresh flock).
	lk, err := Acquire(dir, "push")
	if err != nil {
		t.Fatalf("expected acquire over unheld lock, got %v", err)
	}
	lk.Release()
}
