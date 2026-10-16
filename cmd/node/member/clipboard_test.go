//go:build !echo && !remote

package main

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadClipboardFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(name), 0o600))
		return path
	}
	image := write("shot one.png")
	video := write("clip.MOV")
	text := write("notes.txt")
	imageURI := (&url.URL{Scheme: "file", Path: image}).String()

	files, err := readClipboardFiles(imageURI + "\r\n" + video + "\n" + text + "\nrelative.png\n" + dir)
	require.NoError(t, err)
	require.Len(t, files, 2)
	assert.Equal(t, ClipboardFile{Name: "shot one.png", Type: "image/png", Data: []byte("shot one.png")}, files[0])
	assert.Equal(t, ClipboardFile{Name: "clip.MOV", Type: "video/quicktime", Data: []byte("clip.MOV")}, files[1])

	files, err = readClipboardFiles("https://example.com/a.png")
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestReadClipboardFilesTooLarge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.mp4")
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(maxClipboardFileSize+1))
	require.NoError(t, f.Close())

	_, err = readClipboardFiles(path)
	assert.ErrorIs(t, err, errClipboardFileTooLarge)
}
