/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

//nolint:all
package security

import (
	"bytes"
	"errors"
	"strconv"
	"testing"
	"testing/fstest"

	root "github.com/Warp-net/warpnet"
)

func TestChallengeResolve_Success(t *testing.T) {
	codebase := root.GetCodeBase()
	nonce := int64(1)

	for i := 0; i < 10; i++ {
		sampleOrigin, location, err := GenerateChallenge(codebase, nonce)
		if err != nil {
			t.Fatal(err)
		}
		sampleFound, err := ResolveChallenge(codebase, location, nonce)
		if err != nil {
			t.Fatal(err)
		}

		if string(sampleFound) != string(sampleOrigin) {
			t.Fatalf("samples are not equal: \n %s %s", sampleOrigin, sampleFound)
		}
	}
}

const (
	rootGo = "package main\n\nfunc Root() {}\n"
	aGo    = "package a\n\nfunc A() {}\n"
	bGo    = "package b\n\nfunc B() {}\n"
	cGo    = "package c\n\nfunc C() {}\n"
	dGo    = "package d\n\nfunc D() {}\n"
	zGo    = "package z\n\nfunc Z() {}\n"
)

// sampleTestFS is a tree with files at every depth: the root, intermediate
// directories that also contain subdirectories, and leaves.
func sampleTestFS() fstest.MapFS {
	return fstest.MapFS{
		"rootfile.go":    {Data: []byte(rootGo)},
		"a/afile.go":     {Data: []byte(aGo)},
		"a/b/bfile.go":   {Data: []byte(bGo)},
		"a/b/c/cfile.go": {Data: []byte(cGo)},
		"a/b/d/dfile.go": {Data: []byte(dGo)},
		"x/y/z/zfile.go": {Data: []byte(zGo)},
	}
}

func allContents() map[string]bool {
	return map[string]bool{rootGo: true, aGo: true, bGo: true, cGo: true, dGo: true, zGo: true}
}

// TestGenerateResolve_RoundTrip guards the DirStack/FileStack contract: a
// generated location must resolve back to the same challenge on the same tree.
func TestGenerateResolve_RoundTrip(t *testing.T) {
	codebase := sampleTestFS()
	for i := 0; i < 500; i++ {
		nonce := int64(i)
		gen, loc, err := GenerateChallenge(codebase, nonce)
		if err != nil {
			t.Fatalf("generate (i=%d): %v", i, err)
		}
		res, err := ResolveChallenge(codebase, loc, nonce)
		if err != nil {
			t.Fatalf("resolve (i=%d, loc=%+v): %v", i, loc, err)
		}
		if !bytes.Equal(gen, res) {
			t.Fatalf("round-trip mismatch at i=%d, loc=%+v", i, loc)
		}
	}
}

// TestGenerateSample_WholeFile asserts every sample is a complete file's
// contents, never a substring (regression for the 1-char substring bug).
func TestGenerateSample_WholeFile(t *testing.T) {
	codebase := sampleTestFS()
	contents := allContents()
	for i := 0; i < 500; i++ {
		sample, _, err := generateSample(codebase, ".", []int{})
		if err != nil {
			t.Fatalf("generateSample (i=%d): %v", i, err)
		}
		if !contents[sample] {
			t.Fatalf("sample is not a whole file: %q", sample)
		}
	}
}

// TestGenerateSample_SamplesAllLevels asserts files at every depth get
// sampled, not just leaves (regression for the leaf-biased traversal bug).
func TestGenerateSample_SamplesAllLevels(t *testing.T) {
	codebase := sampleTestFS()
	seen := map[string]bool{}
	for i := 0; i < 5000; i++ {
		sample, _, err := generateSample(codebase, ".", []int{})
		if err != nil {
			t.Fatalf("generateSample (i=%d): %v", i, err)
		}
		seen[sample] = true
	}
	for content := range allContents() {
		if !seen[content] {
			t.Errorf("file at content %q was never sampled - traversal still biased", content)
		}
	}
}

// TestResolveChallenge_WholeFileHash locks in the wire contract: the resolved
// challenge is SHA256(wholeFileContents + nonce).
func TestResolveChallenge_WholeFileHash(t *testing.T) {
	codebase := sampleTestFS()
	loc := SampleLocation{FileStack: []int{0}} // root, first regular file = rootfile.go
	nonce := int64(7)

	got, err := ResolveChallenge(codebase, loc, nonce)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := ConvertToSHA256([]byte(rootGo + strconv.FormatInt(nonce, 10)))
	if !bytes.Equal(got, want) {
		t.Fatalf("challenge is not SHA256(wholeFile + nonce)")
	}
}

func TestResolveChallenge_Deterministic(t *testing.T) {
	codebase := sampleTestFS()
	loc := SampleLocation{FileStack: []int{0}}

	r1, err := ResolveChallenge(codebase, loc, 42)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := ResolveChallenge(codebase, loc, 42)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(r1, r2) {
		t.Fatal("same location and nonce produced different challenges")
	}

	r3, err := ResolveChallenge(codebase, loc, 43)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(r1, r3) {
		t.Fatal("different nonce produced identical challenge")
	}
}

func TestResolveChallenge_Errors(t *testing.T) {
	codebase := sampleTestFS()

	if _, err := ResolveChallenge(codebase, SampleLocation{}, 1); !errors.Is(err, ErrInvalidStackSize) {
		t.Fatalf("empty FileStack: want ErrInvalidStackSize, got %v", err)
	}
	if _, err := ResolveChallenge(codebase, SampleLocation{FileStack: []int{999}}, 1); !errors.Is(err, ErrSampleIndexOutOfBounds) {
		t.Fatalf("file index out of bounds: want ErrSampleIndexOutOfBounds, got %v", err)
	}
	if _, err := ResolveChallenge(codebase, SampleLocation{DirStack: []int{999}, FileStack: []int{0}}, 1); !errors.Is(err, ErrSampleIndexOutOfBounds) {
		t.Fatalf("dir index out of bounds: want ErrSampleIndexOutOfBounds, got %v", err)
	}
	if _, err := ResolveChallenge(codebase, SampleLocation{FileStack: []int{-1}}, 1); !errors.Is(err, ErrSampleIndexOutOfBounds) {
		t.Fatalf("negative file index: want ErrSampleIndexOutOfBounds, got %v", err)
	}
	if _, err := ResolveChallenge(codebase, SampleLocation{DirStack: []int{-1}, FileStack: []int{0}}, 1); !errors.Is(err, ErrSampleIndexOutOfBounds) {
		t.Fatalf("negative dir index: want ErrSampleIndexOutOfBounds, got %v", err)
	}
}

func TestGenerateChallenge_NoFiles(t *testing.T) {
	if _, _, err := GenerateChallenge(fstest.MapFS{}, 1); !errors.Is(err, ErrNoSampleFiles) {
		t.Fatalf("empty codebase: want ErrNoSampleFiles, got %v", err)
	}
}
