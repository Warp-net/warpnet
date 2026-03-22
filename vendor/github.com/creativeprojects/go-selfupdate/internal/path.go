package internal

import (
	"os"
)

// GetExecutablePath returns the path of the executable file with all symlinks resolved.
func GetExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}

	exe, err = ResolvePath(exe)
	if err != nil {
		return "", err
	}

	return exe, nil
}
