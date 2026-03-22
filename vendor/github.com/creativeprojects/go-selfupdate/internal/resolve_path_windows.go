//go:build windows

package internal

import (
	"golang.org/x/sys/windows"
	"os"
	"strings"
	"syscall"
)

// ResolvePath returns the path of a given filename with all symlinks resolved.
func ResolvePath(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Get the Windows handle
	handle := windows.Handle(f.Fd())

	// Probe call to determine the needed buffer size
	bufSize, err := windows.GetFinalPathNameByHandle(handle, nil, 0, 0)
	if err != nil {
		return "", err
	}

	buf := make([]uint16, bufSize)
	n, err := windows.GetFinalPathNameByHandle(handle, &buf[0], uint32(len(buf)), 0)
	if err != nil {
		return "", err
	}

	// Convert the buffer to a string
	final := syscall.UTF16ToString(buf[:n])

	// Strip possible "\\?\" prefix
	final = strings.TrimPrefix(final, `\\?\`)

	return final, nil
}
