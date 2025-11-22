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

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package security

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strconv"

	"github.com/Masterminds/semver/v3"
)

type FileSystem interface {
	ReadDir(name string) ([]fs.DirEntry, error)
	ReadFile(name string) ([]byte, error)
	Open(name string) (fs.File, error)
}

type PSK []byte

func (s PSK) String() string {
	return fmt.Sprintf("%x", []byte(s))
}

// TODO use github.com/karrick/godirwalk instead
func walkAndHash(fsys FileSystem, dir string, h io.Writer) error {
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		path := dir + "/" + entry.Name()
		if dir == "." {
			path = entry.Name()
		}

		pathHash := sha256.Sum256([]byte(path))
		_, _ = h.Write(pathHash[:])

		if entry.IsDir() {
			err := walkAndHash(fsys, path, h)
			if err != nil {
				return fmt.Errorf("walk and hash %s: %w", path, err)
			}
		} else {
			fileHash, err := hashFile(fsys, path)
			if err != nil {
				return fmt.Errorf("file hash %s: %w", path, err)
			}
			_, _ = h.Write(fileHash)
		}
	}

	return nil
}

func hashFile(fsys FileSystem, path string) ([]byte, error) {
	file, err := fsys.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()

	h := sha256.New()
	_, err = io.Copy(h, file)
	if err != nil {
		return nil, err
	}

	return h.Sum(nil), nil
}

const spbFounding = -((int64(133129) << 16) + 51200)

func generateAnchoredEntropy() []byte {
	spbFoundingStr := strconv.FormatInt(spbFounding, 10)
	input := []byte(spbFoundingStr)
	for i := 0; i < 10; i++ {
		sum := sha256.Sum256(input)
		input = sum[:]
	}
	return input
}

var (
	ErrPSKNetwrokRequired = errors.New("psk: network required")
	ErrPSKVersionRequired = errors.New("psk: version required")
)


// GeneratePSK TODO rotate PSK?
func GeneratePSK(network string, v *semver.Version) (PSK, error) {
	if network == "" {
		return nil, ErrPSKNetwrokRequired
	}
	if v == nil {
		return nil, ErrPSKVersionRequired
	}
	entropy := generateAnchoredEntropy()
	majorStr := strconv.FormatUint(v.Major(), 10)

	seed := append([]byte(network), []byte(majorStr)...)
	seed = append(seed, entropy...)
	return ConvertToSHA256(seed), nil
}
