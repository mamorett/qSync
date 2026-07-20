package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// hashBufSize is the streaming read buffer size for hashing (1 MiB).
const hashBufSize = 1 << 20

// HashFile computes the SHA-256 hex digest of the file at path, streaming with a
// 1 MiB buffer so large files never load fully into memory.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	buf := make([]byte, hashBufSize)
	if _, err := io.CopyBuffer(h, f, buf); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// AddHashes computes SHA-256 for every regular file entry in the manifest,
// resolving paths under root. Errors are returned aggregated by the caller;
// this returns the first error encountered.
func AddHashes(m *Manifest, root string) error {
	for _, p := range m.SortedPaths() {
		e := m.Entries[p]
		if e.Type != TypeFile {
			continue
		}
		full := fromSlash(root, p)
		sum, err := HashFile(full)
		if err != nil {
			return err
		}
		e.Hash = sum
		m.Entries[p] = e
	}
	return nil
}
