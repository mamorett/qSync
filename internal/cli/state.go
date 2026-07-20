package cli

import (
	"os"
	"path/filepath"

	"github.com/yourorg/photolib/internal/snapshot"
)

// statePaths returns the manifest file paths for a target library root.
type statePaths struct {
	dir    string
	local  string
	remote string
	synced string
}

func stateFor(targetPath string) statePaths {
	dir := filepath.Join(targetPath, ".photolib", "state")
	return statePaths{
		dir:    dir,
		local:  filepath.Join(dir, "local.manifest.jsonl"),
		remote: filepath.Join(dir, "remote.manifest.jsonl"),
		synced: filepath.Join(dir, "synced.manifest.jsonl"),
	}
}

// loadSynced loads the synced (ancestor) manifest, returning an empty manifest
// when the file is absent (first-run case).
func loadSynced(sp statePaths) (*snapshot.Manifest, bool, error) {
	m, err := snapshot.LoadManifest(sp.synced)
	if err != nil {
		if os.IsNotExist(err) {
			return snapshot.NewManifest("", "", zeroTime()), false, nil
		}
		return nil, false, err
	}
	return m, true, nil
}
