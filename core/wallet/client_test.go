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

//nolint:all
package wallet

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The three platforms disagree about where a user cache lives and about which
// environment variable moves it, so the tests hand the client a directory
// instead of trying to redirect os.UserCacheDir.
func engineIn(t *testing.T, cfg Config) (*Client, string) {
	t.Helper()
	root := t.TempDir()
	client := New(cfg)
	client.cacheDir = func() (string, error) { return root, nil }
	return client, filepath.Join(root, "warpnet")
}

func engineClient(t *testing.T, binary []byte) (*Client, string) {
	t.Helper()
	return engineIn(t, Config{Network: "testnet", BinaryBytes: binary})
}

func unpackedName(binary []byte) string {
	sum := sha256.Sum256(binary)
	return "payment-engine-" + hex.EncodeToString(sum[:6])
}

func TestResolveBinaryUnpacksTheEmbeddedEngine(t *testing.T) {
	binary := []byte("embedded engine bytes")
	client, dir := engineClient(t, binary)
	path, err := client.resolveBinary()
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, unpackedName(binary)) {
		t.Fatalf("path = %s", path)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(binary) {
		t.Fatalf("unpacked %q, err %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("mode = %v", info.Mode())
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %v", info.Mode())
	}
}

func TestResolveBinaryReusesTheSameBytes(t *testing.T) {
	binary := []byte("embedded engine bytes")
	client, dir := engineClient(t, binary)
	path := filepath.Join(dir, unpackedName(binary))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, binary, 0o700); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.resolveBinary()
	if err != nil || got != path {
		t.Fatalf("path = %s, err = %v", got, err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("an identical cached engine was rewritten for nothing")
	}
}

func TestResolveBinaryReplacesTamperedBytesOfTheSameSize(t *testing.T) {
	binary := []byte("embedded engine bytes")
	tampered := []byte("EMBEDDED ENGINE BYTES")
	client, dir := engineClient(t, binary)
	if len(tampered) != len(binary) {
		t.Fatal("the point of this test is a same-size swap")
	}
	path := filepath.Join(dir, unpackedName(binary))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, tampered, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := client.resolveBinary()
	if err != nil || got != path {
		t.Fatalf("path = %s, err = %v", got, err)
	}
	back, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(back) != string(binary) {
		t.Fatalf("the node would have run %q instead of the engine it embeds", back)
	}
}

func TestResolveBinaryRefusesASymlinkedCache(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows symlinks need a privilege and resolve differently; the hash check is what carries there")
	}
	binary := []byte("embedded engine bytes")
	client, dir := engineClient(t, binary)
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(elsewhere, binary, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, unpackedName(binary))
	if err := os.Symlink(elsewhere, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := client.resolveBinary(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("a symlink was left in place of the engine")
	}
}

func TestResolveBinaryPrefersAConfiguredRegularFile(t *testing.T) {
	own := filepath.Join(t.TempDir(), "payment-engine")
	if err := os.WriteFile(own, []byte("operator build"), 0o700); err != nil {
		t.Fatal(err)
	}
	client, _ := engineIn(t, Config{Network: "testnet", BinaryPath: own, BinaryBytes: []byte("embedded")})
	got, err := client.resolveBinary()
	if err != nil || got != own {
		t.Fatalf("path = %s, err = %v", got, err)
	}
}

func TestResolveBinaryIgnoresAConfiguredDirectory(t *testing.T) {
	client, _ := engineIn(t, Config{Network: "testnet", BinaryPath: t.TempDir(), BinaryBytes: []byte("embedded")})
	got, err := client.resolveBinary()
	if err != nil {
		t.Fatal(err)
	}
	if got == client.cfg.BinaryPath {
		t.Fatal("a directory was accepted as the engine")
	}
}

func TestResolveBinaryWithoutAnythingToRun(t *testing.T) {
	client, _ := engineIn(t, Config{Network: "testnet"})
	if _, err := client.resolveBinary(); err == nil {
		t.Fatal("expected an error with no binary path and nothing embedded")
	}
}
