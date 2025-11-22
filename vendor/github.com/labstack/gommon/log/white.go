//go:build appengine
// +build appengine

package log

import (
	"io"
	"os"
)

func output() io.Writer {
	return os.Stdout
}
