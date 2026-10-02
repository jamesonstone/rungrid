package manifest

import (
	"path/filepath"
	"strings"
)

func resolveExisting(filename string) (string, error) {
	abs, err := filepath.Abs(filename)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(filepath.Clean(abs))
}

func within(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
