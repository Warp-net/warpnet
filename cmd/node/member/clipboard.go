//go:build !echo && !remote

package main

import (
	"errors"
	"fmt"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	maxClipboardFiles    = 4
	maxClipboardFileSize = 36 << 20 // the node's video limit, its largest attachment
)

var errClipboardFileTooLarge = errors.New("file is too large to attach")

// mime's built-in table has no video types, so without a system MIME
// database every copied video would be skipped.
var clipboardVideoTypes = map[string]string{
	".mp4": "video/mp4",
	".m4v": "video/x-m4v",
	".mov": "video/quicktime",
}

type ClipboardFile struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Data []byte `json:"data"`
}

// ClipboardFiles returns the images and videos copied in a file manager:
// WebKitGTK pastes only their paths and the webview can't read local files.
func (a *App) ClipboardFiles() ([]ClipboardFile, error) {
	text, err := wailsruntime.ClipboardGetText(a.ctx)
	if err != nil {
		return nil, err
	}
	return readClipboardFiles(text)
}

func readClipboardFiles(text string) ([]ClipboardFile, error) {
	files := []ClipboardFile{}
	for line := range strings.SplitSeq(text, "\n") {
		if len(files) == maxClipboardFiles {
			break
		}
		path := clipboardFilePath(line)
		mimeType := clipboardMediaType(path)
		if path == "" || mimeType == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if info.Size() > maxClipboardFileSize {
			return nil, fmt.Errorf("%w: %s", errClipboardFileTooLarge, info.Name())
		}
		data, err := os.ReadFile(path) //#nosec G304 -- only image and video files the user copied
		if err != nil {
			return nil, err
		}
		files = append(files, ClipboardFile{Name: info.Name(), Type: mimeType, Data: data})
	}
	return files, nil
}

// clipboardFilePath takes both forms file managers copy: a path or a file:// URI.
func clipboardFilePath(line string) string {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "file://") {
		u, err := url.Parse(line)
		if err != nil {
			return ""
		}
		line = u.Path
	}
	if !filepath.IsAbs(line) {
		return ""
	}
	return line
}

func clipboardMediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if t, ok := clipboardVideoTypes[ext]; ok {
		return t
	}
	if t := mime.TypeByExtension(ext); strings.HasPrefix(t, "image/") {
		return t
	}
	return ""
}
