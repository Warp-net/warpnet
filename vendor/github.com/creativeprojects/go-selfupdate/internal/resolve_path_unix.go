//go:build !windows

package internal

import (
	"path/filepath"
)

// ResolvePath returns the path of a given filename with all symlinks resolved.
func ResolvePath(filename string) (string, error) {
	finalPath, err := filepath.EvalSymlinks(filename)
	if err != nil {
		return "", err
	}

	return finalPath, nil
}
