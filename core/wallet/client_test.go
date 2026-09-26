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
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	paymentengine "github.com/Warp-net/payment-engine-lib"
	log "github.com/sirupsen/logrus"
)

// The three platforms disagree about where a user cache lives and about which
// environment variable moves it, so the tests hand the client a directory
// instead of trying to redirect os.UserCacheDir.
func engineIn(t *testing.T, cfg Config, binary []byte) (*Client, string) {
	t.Helper()
	root := t.TempDir()
	client := New(cfg)
	client.cacheDir = func() (string, error) { return root, nil }
	client.engine = func() ([]byte, error) {
		if len(binary) == 0 {
			return nil, paymentengine.ErrUnsupported
		}
		return binary, nil
	}
	return client, filepath.Join(root, "warpnet")
}

func engineClient(t *testing.T, binary []byte) (*Client, string) {
	t.Helper()
	return engineIn(t, Config{Network: "testnet"}, binary)
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
	client, _ := engineIn(t, Config{Network: "testnet", BinaryPath: own}, []byte("embedded"))
	got, err := client.resolveBinary()
	if err != nil || got != own {
		t.Fatalf("path = %s, err = %v", got, err)
	}
}

func TestResolveBinaryIgnoresAConfiguredDirectory(t *testing.T) {
	client, _ := engineIn(t, Config{Network: "testnet", BinaryPath: t.TempDir()}, []byte("embedded"))
	got, err := client.resolveBinary()
	if err != nil {
		t.Fatal(err)
	}
	if got == client.cfg.BinaryPath {
		t.Fatal("a directory was accepted as the engine")
	}
}

func TestResolveBinaryWithoutAnythingToRun(t *testing.T) {
	client, _ := engineIn(t, Config{Network: "testnet"}, nil)
	if _, err := client.resolveBinary(); err == nil {
		t.Fatal("expected an error with no binary path and nothing embedded")
	}
}

func TestEngineArgumentsAreOnesTheEngineAccepts(t *testing.T) {
	accepted := map[string]bool{
		"-mainnet-endpoint": true, "-testnet-endpoint": true,
		"-mainnet-registry": true, "-testnet-registry": true,
		"-network": true, "-rps": true, "-timeout": true,
		"-confirmations": true, "-request-timeout": true, "-contract": true,
	}
	client, _ := engineIn(t, Config{Network: "testnet", Endpoint: "https://nile.trongrid.io", RPS: 2}, nil)
	args := client.engineArgs()
	if args[0] != "serve" {
		t.Fatalf("args = %v, want serve first", args)
	}
	for _, arg := range args {
		if len(arg) > 1 && arg[0] == '-' && !accepted[arg] {
			t.Fatalf("the engine does not define %s: it prints that on stderr and exits, and args = %v", arg, args)
		}
	}
}

func TestEngineStderrReachesTheLog(t *testing.T) {
	var logged bytes.Buffer
	previous := log.StandardLogger().Out
	log.SetOutput(&logged)
	defer log.SetOutput(previous)

	client, _ := engineIn(t, Config{Network: "testnet"}, nil)
	refusal := "flag provided but not defined: -api-key\n\nUsage of payment-engine serve:\n"
	client.readErrors(strings.NewReader(refusal))

	if !strings.Contains(logged.String(), "flag provided but not defined: -api-key") {
		t.Fatalf("the engine's reason never reached the log: %q", logged.String())
	}
	if got := strings.Count(logged.String(), "payment engine:"); got != 2 {
		t.Fatalf("logged %d lines, want the two non-empty ones: %q", got, logged.String())
	}
}

func TestPayParamsCarryEveryFieldTheEngineRequires(t *testing.T) {
	sponsorship := Sponsorship{
		Splitter:      "TBuRiiib6EqsezQMMihnBsq2wrAZscxbDy",
		OrderId:       "0x0000000000000000000000000000000000000000000000000000000000000001",
		Author:        "THXiCmfr6D4mqAfd4La9EQ5THCx7WsR143",
		Amount:        "1000000",
		MaxFeePercent: 5,
	}
	params := payParams("testnet", "abcdef", sponsorship)
	for _, key := range []string{"splitter", "order_id", "author", "amount", "network", "seed", "max_fee_percent"} {
		if _, ok := params[key]; !ok {
			t.Fatalf("wallet.pay needs %q, got %v", key, params)
		}
	}
	if len(params) != 7 {
		t.Fatalf("wallet.pay was sent something it does not define: %v", params)
	}

	sponsorship.MaxFeePercent = 0
	if _, ok := payParams("testnet", "abcdef", sponsorship)["max_fee_percent"]; ok {
		t.Fatal("a sponsorship that agreed no ceiling must not send one")
	}
}
