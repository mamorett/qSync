package purge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PendingDeletion is one staged deletion awaiting purge.
type PendingDeletion struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// PendingSet is the on-disk pending-deletions.json payload.
type PendingSet struct {
	Host      string            `json:"host"`      // remote host deletions target
	Path      string            `json:"path"`      // remote root path
	Staged    string            `json:"staged"`    // RFC3339 timestamp
	Deletions []PendingDeletion `json:"deletions"` // sorted by Path
}

// PendingPath returns the path of the pending-deletions file for a target.
func PendingPath(targetPath string) string {
	return filepath.Join(targetPath, ".qsync", "state", "pending-deletions.json")
}

// LoadPending reads the pending-deletions file. Returns an empty set (not an
// error) when the file is absent.
func LoadPending(targetPath string) (*PendingSet, error) {
	path := PendingPath(targetPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &PendingSet{}, nil
		}
		return nil, err
	}
	var ps PendingSet
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, fmt.Errorf("parse pending deletions: %w", err)
	}
	return &ps, nil
}

// SavePending atomically writes the pending-deletions file.
func (ps *PendingSet) Save(targetPath string) error {
	sort.Slice(ps.Deletions, func(i, j int) bool { return ps.Deletions[i].Path < ps.Deletions[j].Path })
	path := PendingPath(targetPath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Clear removes the pending-deletions file (after a successful purge).
func Clear(targetPath string) error {
	err := os.Remove(PendingPath(targetPath))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Count returns the number of staged deletions.
func (ps *PendingSet) Count() int { return len(ps.Deletions) }

// ConfirmPhrase is the exact phrase a user must type to confirm.
func ConfirmPhrase(n int) string {
	return fmt.Sprintf("delete %d files", n)
}

// ShellQuote single-quotes a path for safe use in a remote shell command.
// Paths containing newlines are rejected by the caller (they break manifests).
func ShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\r'\"\\$`*?[]{}();&|<>#~!") {
		return s
	}
	// Wrap in single quotes; escape embedded single quotes.
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// chunkPaths splits paths into chunks of at most size elements.
func chunkPaths(paths []string, size int) [][]string {
	var chunks [][]string
	for i := 0; i < len(paths); i += size {
		end := i + size
		if end > len(paths) {
			end = len(paths)
		}
		chunks = append(chunks, paths[i:end])
	}
	return chunks
}

// RemoteDeleteCommands builds the ssh rm commands (chunked at 500 paths) to
// delete the staged paths on the remote host under root. Returns argv slices
// (each ready to pass to exec after the ssh binary). Paths with newlines cause
// an error.
func RemoteDeleteCommands(sshBin, host, root string, paths []string, chunkSize int) ([][]string, error) {
	for _, p := range paths {
		if strings.ContainsAny(p, "\n\r") {
			return nil, fmt.Errorf("path contains newline; cannot purge safely: %q", p)
		}
	}
	if chunkSize <= 0 {
		chunkSize = 500
	}
	sort.Strings(paths)
	var cmds [][]string
	for _, chunk := range chunkPaths(paths, chunkSize) {
		var b strings.Builder
		b.WriteString("rm -f --")
		for _, p := range chunk {
			full := root + "/" + p
			b.WriteString(" ")
			b.WriteString(ShellQuote(full))
		}
		cmds = append(cmds, []string{host, b.String()})
	}
	return cmds, nil
}
