package gosod

import (
	"io/fs"

	"github.com/leaanthony/gosod/internal/templatedir"
)

// New creates a new TemplateDir structure for the given filesystem
func New(fs fs.FS) *templatedir.TemplateDir {
	return templatedir.New(fs)
}
