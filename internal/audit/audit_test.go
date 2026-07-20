package audit

import (
	"path/filepath"
	"testing"
	"time"
)

func TestWriterRoundTrip(t *testing.T) {
	dir := t.TempDir()
	when := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	w, err := NewWriter(dir, "pull", when)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append(Record{Operation: "pull", FilesChanged: 2, Result: "success"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Append(Record{Operation: "pull", Path: "a.jpg", Result: "received"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	name := "20260720-100000-pull.jsonl"
	recs, err := ReadFile(filepath.Join(HistoryDir(dir), name))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	if recs[0].FilesChanged != 2 || recs[1].Path != "a.jpg" {
		t.Errorf("records wrong: %+v", recs)
	}
	if recs[0].Hostname == "" {
		t.Error("hostname should be auto-filled")
	}
}
