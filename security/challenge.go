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

package security

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2" //#nosec
	"path"
	"sort"
	"strconv"
)

type SampleLocation struct {
	DirStack  []int // every index is level and value is dir num
	FileStack []int // file index
}

func ResolveChallenge(codebase FileSystem, location SampleLocation, nonce int64) ([]byte, error) {
	sample, err := findSample(codebase, location)
	if err != nil {
		return nil, err
	}
	challengeResult := ConvertToSHA256([]byte(sample + strconv.FormatInt(nonce, 10)))
	return challengeResult, nil
}

func GenerateChallenge(codebase FileSystem, nonce int64) ([]byte, SampleLocation, error) {
	sample, location, err := generateSample(codebase, ".", []int{})
	if err != nil {
		return nil, SampleLocation{}, err
	}
	challengeResult := ConvertToSHA256([]byte(sample + strconv.FormatInt(nonce, 10)))
	return challengeResult, location, nil
}

var (
	ErrNoSampleFiles          = errors.New("challenge: no usable files or subdirectories")
	ErrSampleIndexOutOfBounds = errors.New("sample index out of bounds")
)

func generateSample(codebase FileSystem, dir string, dirStack []int) (_ string, result SampleLocation, err error) {
	entries, err := fs.ReadDir(codebase, dir)
	if err != nil {
		return "", result, fmt.Errorf("challenge: read dir %s: %w", dir, err)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var dirs, files []fs.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}

	// Subdirectories and the files of this directory compete on equal footing,
	// so files outside leaf directories are sampled too.
	total := len(dirs) + len(files)
	for _, c := range rand.Perm(total) {
		if c < len(dirs) {
			subPath := path.Join(dir, dirs[c].Name())
			sample, subResult, err := generateSample(codebase, subPath, append(dirStack, c))
			if err == nil {
				return sample, subResult, nil
			}
			continue
		}

		fileIndex := c - len(dirs)
		fullPath := path.Join(dir, files[fileIndex].Name())

		content, err := fs.ReadFile(codebase, fullPath)
		if err != nil || len(content) == 0 {
			continue
		}

		return string(content), SampleLocation{
			DirStack:  dirStack,
			FileStack: []int{fileIndex},
		}, nil
	}

	return "", result, ErrNoSampleFiles
}

var ErrInvalidStackSize = errors.New("challenge: invalid file stack size - expected at least 1 element")

func findSample(codebase FileSystem, loc SampleLocation) (string, error) {
	currentDir := "."

	for level, dirIndex := range loc.DirStack {
		entries, err := fs.ReadDir(codebase, currentDir)
		if err != nil {
			return "", fmt.Errorf("challenge: read dir %s: %w", currentDir, err)
		}

		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].Name() < entries[j].Name()
		})

		var dirs []fs.DirEntry
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, e)
			}
		}
		if dirIndex < 0 || dirIndex >= len(dirs) {
			return "", fmt.Errorf("challenge: dir index %d: level %d: %w", dirIndex, level, ErrSampleIndexOutOfBounds)
		}

		nextDir := dirs[dirIndex].Name()
		currentDir = path.Join(currentDir, nextDir)
	}

	entries, err := fs.ReadDir(codebase, currentDir)
	if err != nil {
		return "", fmt.Errorf("challenge: read files from %s: %w", currentDir, err)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var regularFiles []fs.DirEntry
	for _, e := range entries {
		if !e.IsDir() {
			regularFiles = append(regularFiles, e)
		}
	}
	if len(loc.FileStack) < 1 {
		return "", ErrInvalidStackSize
	}

	fileIndex := loc.FileStack[0]
	if fileIndex < 0 || fileIndex >= len(regularFiles) {
		return "", fmt.Errorf("challenge: %d %w - found %d files", fileIndex, ErrSampleIndexOutOfBounds, len(regularFiles))
	}

	targetFile := regularFiles[fileIndex].Name()
	fullPath := path.Join(currentDir, targetFile)

	content, err := fs.ReadFile(codebase, fullPath)
	if err != nil {
		return "", fmt.Errorf("challenge: read file %s: %w", fullPath, err)
	}

	return string(content), nil
}
