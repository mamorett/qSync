package snapshot

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if content == "@dir" {
			if err := os.MkdirAll(full, 0755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if len(content) > len("@symlink:") && content[:len("@symlink:")] == "@symlink:" {
			target := content[len("@symlink:"):]
			os.MkdirAll(filepath.Dir(full), 0755)
			if err := os.Symlink(target, full); err != nil {
				t.Fatal(err)
			}
			continue
		}
		os.MkdirAll(filepath.Dir(full), 0755)
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestScan_BasicAndSkipQsync(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"2024/05/a.jpg":  "hello",
		"2024/05/b.jpg":  "world",
		".qsync/state/x": "state",
		"link":           "@symlink:2024/05/a.jpg",
	})
	res, err := Scan(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := res.Manifest
	if _, ok := m.Entries["2024/05/a.jpg"]; !ok {
		t.Error("missing a.jpg")
	}
	if _, ok := m.Entries[".qsync/state/x"]; ok {
		t.Error(".qsync should be skipped")
	}
	if e, ok := m.Entries["link"]; !ok || e.Type != TypeSymlink {
		t.Errorf("symlink not recorded properly: %+v", e)
	}
	if e := m.Entries["2024"]; e.Type != TypeDir {
		t.Error("directory 2024 should be TypeDir")
	}
}

func TestScan_IgnorePatterns(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"keep.jpg":      "x",
		"Thumbs.db":     "x",
		"cache/big.tmp": "x",
		"2024/skip.tmp": "x",
	})
	res, err := Scan(root, []string{"Thumbs.db", "*.tmp", "cache/"})
	if err != nil {
		t.Fatal(err)
	}
	m := res.Manifest
	if _, ok := m.Entries["keep.jpg"]; !ok {
		t.Error("keep.jpg should be present")
	}
	if _, ok := m.Entries["Thumbs.db"]; ok {
		t.Error("Thumbs.db should be ignored")
	}
	if _, ok := m.Entries["2024/skip.tmp"]; ok {
		t.Error("*.tmp should be ignored anywhere")
	}
	if _, ok := m.Entries["cache"]; ok {
		t.Error("cache/ dir should be ignored")
	}
}

func TestManifestRoundTrip(t *testing.T) {
	m := NewManifest("host1", "/photos", time.Unix(1716105600, 0).UTC())
	m.Entries["b"] = Entry{Path: "b", Size: 2, ModTime: 200, Mode: 0644, Type: TypeFile}
	m.Entries["a"] = Entry{Path: "a", Size: 1, ModTime: 100, Mode: 0644, Type: TypeFile}

	var buf bytes.Buffer
	if err := m.Write(&buf); err != nil {
		t.Fatal(err)
	}
	// Second entry line must be "a" (sorted).
	got, err := Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != "host1" || got.Root != "/photos" {
		t.Errorf("header lost: %+v", got)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got.Entries))
	}
	if got.Entries["a"].Size != 1 {
		t.Errorf("entry a wrong: %+v", got.Entries["a"])
	}
}

func TestManifestWriteDeterministic(t *testing.T) {
	m := NewManifest("h", "/r", time.Unix(0, 0).UTC())
	for _, p := range []string{"z", "a", "m", "b"} {
		m.Entries[p] = Entry{Path: p, Type: TypeFile}
	}
	var b1, b2 bytes.Buffer
	m.Write(&b1)
	m.Write(&b2)
	if !bytes.Equal(b1.Bytes(), b2.Bytes()) {
		t.Fatal("manifest write not deterministic")
	}
}

func TestManifestDiff(t *testing.T) {
	base := NewManifest("h", "/r", time.Time{})
	base.Entries["same"] = Entry{Path: "same", Size: 1, ModTime: 1, Type: TypeFile}
	base.Entries["changed"] = Entry{Path: "changed", Size: 1, ModTime: 1, Type: TypeFile}
	base.Entries["removed"] = Entry{Path: "removed", Size: 1, ModTime: 1, Type: TypeFile}

	other := NewManifest("h", "/r", time.Time{})
	other.Entries["same"] = Entry{Path: "same", Size: 1, ModTime: 1, Type: TypeFile}
	other.Entries["changed"] = Entry{Path: "changed", Size: 2, ModTime: 1, Type: TypeFile}
	other.Entries["added"] = Entry{Path: "added", Size: 1, ModTime: 1, Type: TypeFile}

	d := base.Diff(other)
	if len(d.Added) != 1 || d.Added[0] != "added" {
		t.Errorf("added wrong: %v", d.Added)
	}
	if len(d.Updated) != 1 || d.Updated[0] != "changed" {
		t.Errorf("updated wrong: %v", d.Updated)
	}
	if len(d.Deleted) != 1 || d.Deleted[0] != "removed" {
		t.Errorf("deleted wrong: %v", d.Deleted)
	}
}

func TestHashFile(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "f")
	os.WriteFile(p, []byte("abc"), 0644)
	h, err := HashFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// sha256("abc")
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if h != want {
		t.Errorf("hash = %s, want %s", h, want)
	}
}
