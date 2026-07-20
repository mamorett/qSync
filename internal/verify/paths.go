package verify

import "path/filepath"

func fromSlash(root, rel string) string {
	return filepath.Join(root, filepath.FromSlash(rel))
}
