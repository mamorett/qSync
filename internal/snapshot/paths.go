package snapshot

import "path/filepath"

// fromSlash joins a slash-separated relative manifest path onto an OS root path.
func fromSlash(root, rel string) string {
	return filepath.Join(root, filepath.FromSlash(rel))
}
