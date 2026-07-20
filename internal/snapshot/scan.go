package snapshot

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// ScanError records a per-entry failure during a scan.
type ScanError struct {
	Path string
	Err  error
}

func (e ScanError) Error() string {
	return fmt.Sprintf("%s: %v", e.Path, e.Err)
}

// ScanResult bundles a manifest with any non-fatal per-file errors.
type ScanResult struct {
	Manifest *Manifest
	Errors   []ScanError
}

// nowFunc is overridable in tests; defaults to time.Now.
var nowFunc = time.Now

// hostnameFunc is overridable in tests.
var hostnameFunc = os.Hostname

// Scan walks root and builds a manifest. The .qsync directory at the root is
// skipped. Entries matching any ignore pattern are skipped. Per-file errors are
// collected rather than aborting the whole scan.
func Scan(root string, ignore []string) (*ScanResult, error) {
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("scan root %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("scan root %s: not a directory", root)
	}

	host, _ := hostnameFunc()
	m := NewManifest(host, root, nowFunc())
	res := &ScanResult{Manifest: m}

	patterns := compileIgnore(ignore)

	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// Directory read error; record and skip.
			if p == root {
				return err
			}
			res.Errors = append(res.Errors, ScanError{Path: relSlash(root, p), Err: err})
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if p == root {
			return nil
		}

		rel := relSlash(root, p)

		// Skip the .qsync state dir at the root entirely.
		if rel == ".qsync" {
			return fs.SkipDir
		}

		isDir := d.IsDir()
		if patterns.match(rel, isDir) {
			if isDir {
				return fs.SkipDir
			}
			return nil
		}

		// Reject paths with newlines: they break JSONL manifests.
		if strings.ContainsAny(rel, "\n\r") {
			res.Errors = append(res.Errors, ScanError{Path: rel, Err: fmt.Errorf("path contains newline; unsupported")})
			if isDir {
				return fs.SkipDir
			}
			return nil
		}

		fi, err := d.Info()
		if err != nil {
			res.Errors = append(res.Errors, ScanError{Path: rel, Err: err})
			return nil
		}

		e := Entry{
			Path:    rel,
			ModTime: fi.ModTime().Truncate(time.Second).Unix(),
			Mode:    uint32(fi.Mode().Perm()),
		}

		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			target, lerr := os.Readlink(p)
			if lerr != nil {
				res.Errors = append(res.Errors, ScanError{Path: rel, Err: lerr})
				return nil
			}
			e.Type = TypeSymlink
			e.LinkTarget = filepath.ToSlash(target)
			e.Size = 0
		case isDir:
			e.Type = TypeDir
			e.Size = fi.Size()
		default:
			e.Type = TypeFile
			e.Size = fi.Size()
		}

		m.Entries[rel] = e
		return nil
	})
	if walkErr != nil {
		return res, fmt.Errorf("scan %s: %w", root, walkErr)
	}
	return res, nil
}

func relSlash(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

// ignorePattern is a compiled gitignore-style pattern (supported subset).
type ignorePattern struct {
	raw      string
	dirOnly  bool // trailing /
	anchored bool // leading /
	base     string
}

type ignoreSet struct {
	patterns []ignorePattern
}

func compileIgnore(patterns []string) *ignoreSet {
	set := &ignoreSet{}
	for _, raw := range patterns {
		p := strings.TrimSpace(raw)
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		ip := ignorePattern{raw: p}
		if strings.HasSuffix(p, "/") {
			ip.dirOnly = true
			p = strings.TrimSuffix(p, "/")
		}
		if strings.HasPrefix(p, "/") {
			ip.anchored = true
			p = strings.TrimPrefix(p, "/")
		}
		ip.base = p
		set.patterns = append(set.patterns, ip)
	}
	return set
}

// match reports whether the relative slash path (isDir marks directories)
// matches any ignore pattern.
//
// Supported subset:
//   - literal names ("Thumbs.db")
//   - '*' globs via path.Match (single path segment)
//   - trailing '/' matches directories only
//   - leading '/' anchors the pattern to the library root
//
// Unanchored patterns match against any path component or the full path.
func (s *ignoreSet) match(rel string, isDir bool) bool {
	for _, ip := range s.patterns {
		if ip.dirOnly && !isDir {
			continue
		}
		if ip.anchored {
			if matchGlob(ip.base, rel) {
				return true
			}
			continue
		}
		// Unanchored: match full path or the basename or any segment.
		if matchGlob(ip.base, rel) {
			return true
		}
		if matchGlob(ip.base, path.Base(rel)) {
			return true
		}
		// Match against each path segment for bare-name patterns.
		if !strings.ContainsAny(ip.base, "/") {
			for _, seg := range strings.Split(rel, "/") {
				if matchGlob(ip.base, seg) {
					return true
				}
			}
		}
	}
	return false
}

func matchGlob(pattern, name string) bool {
	if !strings.ContainsAny(pattern, "*?[") {
		return pattern == name
	}
	ok, err := path.Match(pattern, name)
	if err != nil {
		return false
	}
	return ok
}
