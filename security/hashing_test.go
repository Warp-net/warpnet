package security

import (
	"crypto/sha256"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConvertToSHA256_Empty(t *testing.T) {
	result := ConvertToSHA256([]byte{})
	assert.Empty(t, result)
}

func TestConvertToSHA256_Nil(t *testing.T) {
	result := ConvertToSHA256(nil)
	assert.Empty(t, result)
}

func TestConvertToSHA256_ValidInput(t *testing.T) {
	input := []byte("hello world")
	result := ConvertToSHA256(input)

	expected := sha256.New()
	expected.Write(input)
	assert.Equal(t, expected.Sum(nil), result)
	assert.Len(t, result, 32)
}

func TestConvertToSHA256_Deterministic(t *testing.T) {
	input := []byte("deterministic test")
	r1 := ConvertToSHA256(input)
	r2 := ConvertToSHA256(input)
	assert.Equal(t, r1, r2)
}

func TestConvertToSHA256_DifferentInputs(t *testing.T) {
	r1 := ConvertToSHA256([]byte("input1"))
	r2 := ConvertToSHA256([]byte("input2"))
	assert.NotEqual(t, r1, r2)
}

type testFS struct {
	fstest.MapFS
}

func (t *testFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(t.MapFS, name)
}

func (t *testFS) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(t.MapFS, name)
}

func (t *testFS) Open(name string) (fs.File, error) {
	return t.MapFS.Open(name)
}

func TestGetCodebaseHashHex(t *testing.T) {
	tfs := &testFS{
		MapFS: fstest.MapFS{
			"file1.go": &fstest.MapFile{
				Data:    []byte("package main"),
				ModTime: time.Now(),
			},
			"file2.go": &fstest.MapFile{
				Data:    []byte("package test"),
				ModTime: time.Now(),
			},
		},
	}

	hash, err := GetCodebaseHashHex(tfs)
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.Len(t, hash, 64) // hex-encoded SHA256
}

func TestGetCodebaseHashHex_Deterministic(t *testing.T) {
	tfs := &testFS{
		MapFS: fstest.MapFS{
			"a.go": &fstest.MapFile{Data: []byte("content")},
		},
	}

	h1, err1 := GetCodebaseHashHex(tfs)
	h2, err2 := GetCodebaseHashHex(tfs)
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.Equal(t, h1, h2)
}

func TestGetCodebaseHashHex_DifferentContent(t *testing.T) {
	tfs1 := &testFS{
		MapFS: fstest.MapFS{
			"a.go": &fstest.MapFile{Data: []byte("content1")},
		},
	}
	tfs2 := &testFS{
		MapFS: fstest.MapFS{
			"a.go": &fstest.MapFile{Data: []byte("content2")},
		},
	}

	h1, _ := GetCodebaseHashHex(tfs1)
	h2, _ := GetCodebaseHashHex(tfs2)
	assert.NotEqual(t, h1, h2)
}

func TestGetCodebaseHashHex_WithSubdirectory(t *testing.T) {
	tfs := &testFS{
		MapFS: fstest.MapFS{
			"dir/file.go": &fstest.MapFile{Data: []byte("nested")},
		},
	}

	hash, err := GetCodebaseHashHex(tfs)
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
}
