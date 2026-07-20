package verify

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/yourorg/photolib/internal/snapshot"
)

// Mismatch is one verification failure.
type Mismatch struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"` // missing-local | missing-remote | size | mtime | checksum
	Expect string `json:"expect"`
	Got    string `json:"got"`
}

// Result is the verification outcome.
type Result struct {
	Checked    int        `json:"checked"`
	Mismatches []Mismatch `json:"mismatches"`
}

// OK reports whether verification passed (no mismatches).
func (r *Result) OK() bool { return len(r.Mismatches) == 0 }

// Verify compares local and remote manifests. In default mode it compares
// size + mtime(seconds). In checksum mode, for pairs that pass size comparison
// it computes the local file's SHA-256 (streaming) and compares against the
// remote manifest's Hash field. root is the local library root.
func Verify(local, remote *snapshot.Manifest, checksum bool, root string) (*Result, error) {
	res := &Result{}

	// Union of paths for missing detection.
	paths := map[string]struct{}{}
	for p := range local.Entries {
		paths[p] = struct{}{}
	}
	for p := range remote.Entries {
		paths[p] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for p := range paths {
		ordered = append(ordered, p)
	}
	sort.Strings(ordered)

	for _, p := range ordered {
		le, lOK := local.Entries[p]
		re, rOK := remote.Entries[p]

		if !lOK {
			res.Mismatches = append(res.Mismatches, Mismatch{Path: p, Kind: "missing-local", Expect: "present", Got: "absent"})
			continue
		}
		if !rOK {
			res.Mismatches = append(res.Mismatches, Mismatch{Path: p, Kind: "missing-remote", Expect: "present", Got: "absent"})
			continue
		}

		res.Checked++

		// Skip content comparison for dirs/symlinks beyond type.
		if le.Type != re.Type {
			res.Mismatches = append(res.Mismatches, Mismatch{Path: p, Kind: "size", Expect: fmt.Sprintf("type %d", re.Type), Got: fmt.Sprintf("type %d", le.Type)})
			continue
		}
		if le.Type != snapshot.TypeFile {
			continue
		}

		if le.Size != re.Size {
			res.Mismatches = append(res.Mismatches, Mismatch{Path: p, Kind: "size", Expect: strconv.FormatInt(re.Size, 10), Got: strconv.FormatInt(le.Size, 10)})
			continue
		}
		if le.ModTime != re.ModTime {
			res.Mismatches = append(res.Mismatches, Mismatch{Path: p, Kind: "mtime", Expect: strconv.FormatInt(re.ModTime, 10), Got: strconv.FormatInt(le.ModTime, 10)})
			continue
		}

		if checksum {
			sum, err := snapshot.HashFile(fromSlash(root, p))
			if err != nil {
				return nil, err
			}
			if re.Hash != "" && sum != re.Hash {
				res.Mismatches = append(res.Mismatches, Mismatch{Path: p, Kind: "checksum", Expect: re.Hash, Got: sum})
			}
		}
	}

	sort.Slice(res.Mismatches, func(i, j int) bool { return res.Mismatches[i].Path < res.Mismatches[j].Path })
	return res, nil
}
