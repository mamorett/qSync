package snapshot

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// FileType enumerates the kinds of entries recorded in a manifest.
type FileType int

const (
	// TypeFile is a regular file.
	TypeFile FileType = iota
	// TypeDir is a directory.
	TypeDir
	// TypeSymlink is a symbolic link.
	TypeSymlink
)

// ManifestVersion is the current on-disk schema version.
const ManifestVersion = 1

// Entry is a single filesystem object recorded in a manifest.
type Entry struct {
	Path       string   `json:"path"`
	Size       int64    `json:"size"`
	ModTime    int64    `json:"mtime"` // Unix seconds
	Mode       uint32   `json:"mode"`  // permission bits only (os.FileMode & 0777)
	Type       FileType `json:"type"`
	LinkTarget string   `json:"link_target,omitempty"`
	Hash       string   `json:"hash,omitempty"`
}

// Manifest is a snapshot of one side of the library.
type Manifest struct {
	Version   int
	Generated time.Time
	Host      string
	Root      string
	Entries   map[string]Entry
}

// header is the first JSONL line of a manifest file.
type header struct {
	Schema    int    `json:"photolib-manifest"`
	Generated string `json:"generated"`
	Host      string `json:"host"`
	Root      string `json:"root"`
}

// NewManifest returns an empty manifest ready for population.
func NewManifest(host, root string, generated time.Time) *Manifest {
	return &Manifest{
		Version:   ManifestVersion,
		Generated: generated,
		Host:      host,
		Root:      root,
		Entries:   make(map[string]Entry),
	}
}

// SortedPaths returns manifest paths sorted byte-wise ascending.
func (m *Manifest) SortedPaths() []string {
	paths := make([]string, 0, len(m.Entries))
	for p := range m.Entries {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// Write serializes the manifest as JSON Lines to w.
func (m *Manifest) Write(w io.Writer) error {
	bw := bufio.NewWriter(w)
	h := header{
		Schema:    ManifestVersion,
		Generated: m.Generated.UTC().Format(time.RFC3339),
		Host:      m.Host,
		Root:      m.Root,
	}
	hb, err := json.Marshal(h)
	if err != nil {
		return err
	}
	if _, err := bw.Write(hb); err != nil {
		return err
	}
	if err := bw.WriteByte('\n'); err != nil {
		return err
	}
	for _, p := range m.SortedPaths() {
		eb, err := json.Marshal(m.Entries[p])
		if err != nil {
			return err
		}
		if _, err := bw.Write(eb); err != nil {
			return err
		}
		if err := bw.WriteByte('\n'); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// Save writes the manifest atomically to path (write to .tmp then rename).
func (m *Manifest) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if err := m.Write(f); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// Read parses a manifest from r.
func Read(r io.Reader) (*Manifest, error) {
	sc := bufio.NewScanner(r)
	// Allow long lines (paths + metadata).
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	m := &Manifest{Version: ManifestVersion, Entries: make(map[string]Entry)}
	first := true
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		if first {
			var h header
			if err := json.Unmarshal(line, &h); err != nil {
				return nil, fmt.Errorf("parse manifest header: %w", err)
			}
			m.Version = h.Schema
			m.Host = h.Host
			m.Root = h.Root
			if h.Generated != "" {
				if t, err := time.Parse(time.RFC3339, h.Generated); err == nil {
					m.Generated = t
				}
			}
			first = false
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("parse manifest entry: %w", err)
		}
		m.Entries[e.Path] = e
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if first {
		return nil, fmt.Errorf("empty manifest")
	}
	return m, nil
}

// LoadManifest reads a manifest from a file path.
func LoadManifest(path string) (*Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f)
}

// ManifestDiff describes the differences between two manifests.
type ManifestDiff struct {
	Added   []string // present in other, absent in receiver (from receiver's POV, these are new)
	Updated []string // present in both, content differs
	Deleted []string // present in receiver, absent in other
}

// entryContentDiffers reports whether two entries differ in a way that matters
// for change detection: existence, size, mtime (seconds), or type. Mode and
// hash are intentionally ignored here.
func entryContentDiffers(a, b Entry) bool {
	if a.Type != b.Type {
		return true
	}
	if a.Type == TypeSymlink {
		return a.LinkTarget != b.LinkTarget
	}
	if a.Type == TypeDir {
		// Directories: existence/type only; size/mtime of dirs are noisy.
		return false
	}
	return a.Size != b.Size || a.ModTime != b.ModTime
}

// Diff compares the receiver (base) against other and reports how other differs
// from the receiver. Added = in other but not base; Updated = in both but
// content differs; Deleted = in base but not other. All slices sorted.
func (m *Manifest) Diff(other *Manifest) *ManifestDiff {
	d := &ManifestDiff{}
	for p, oe := range other.Entries {
		be, ok := m.Entries[p]
		if !ok {
			d.Added = append(d.Added, p)
			continue
		}
		if entryContentDiffers(be, oe) {
			d.Updated = append(d.Updated, p)
		}
	}
	for p := range m.Entries {
		if _, ok := other.Entries[p]; !ok {
			d.Deleted = append(d.Deleted, p)
		}
	}
	sort.Strings(d.Added)
	sort.Strings(d.Updated)
	sort.Strings(d.Deleted)
	return d
}
