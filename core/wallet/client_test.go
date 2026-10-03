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
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	paymentengine "github.com/Warp-net/payment-engine-lib"
	"github.com/Warp-net/warpnet/json"
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

func answeringEngine(t *testing.T, answer func(request) string) *Client {
	t.Helper()
	client, _ := engineClient(t, nil)
	requests, requestsW := io.Pipe()
	answers, answersW := io.Pipe()
	engine := &exec.Cmd{}
	client.cmd, client.stop, client.stdin = engine, func() {}, requestsW
	go client.read(engine, answers)
	go func() {
		scanner := bufio.NewScanner(requests)
		for scanner.Scan() {
			var req request
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
				return
			}
			line := answer(req)
			if line == "" {
				_ = answersW.Close()
				return
			}
			_, _ = answersW.Write([]byte(line + "\n"))
		}
	}()
	t.Cleanup(func() {
		_ = answersW.Close()
		_ = requestsW.Close()
	})
	return client
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

func TestEngineStoppedOnPurposeIsNotReported(t *testing.T) {
	var logged bytes.Buffer
	previous := log.StandardLogger().Out
	log.SetOutput(&logged)
	defer log.SetOutput(previous)

	client, _ := engineClient(t, nil)
	waiting := make(chan response, 1)
	client.pending["1"] = waiting

	client.read(&exec.Cmd{}, strings.NewReader(""))

	if logged.Len() != 0 {
		t.Fatalf("a closed engine was reported as gone: %q", logged.String())
	}
	select {
	case resp := <-waiting:
		if resp.Error == nil {
			t.Fatal("a request waiting on a closed engine was answered without an error")
		}
	default:
		t.Fatal("a request waiting on a closed engine was never answered")
	}
}

func TestEngineThatDiesIsReportedOnce(t *testing.T) {
	var logged bytes.Buffer
	previous := log.StandardLogger().Out
	log.SetOutput(&logged)
	defer log.SetOutput(previous)

	client, _ := engineClient(t, nil)
	engine := &exec.Cmd{}
	isStopped := false
	client.cmd, client.stop = engine, func() { isStopped = true }

	client.read(engine, strings.NewReader(""))

	if got := strings.Count(logged.String(), "payment engine unavailable"); got != 1 {
		t.Fatalf("logged the failure %d times, want once: %q", got, logged.String())
	}
	if !isStopped || client.cmd != nil || !errors.Is(client.failure, ErrUnavailable) {
		t.Fatalf("the dead engine is still held: stopped=%v cmd=%v failure=%v", isStopped, client.cmd, client.failure)
	}
}

func TestEngineAnswersBeforeTheClientGivesUp(t *testing.T) {
	client, _ := engineClient(t, nil)
	args := strings.Join(client.engineArgs(), " ")
	if !strings.Contains(args, "-timeout "+engineTimeout.String()) || engineTimeout >= requestTimeout {
		t.Fatalf("args = %q: the engine must give up on a request before the client stops waiting for it", args)
	}
}

func TestPayKeepsTheTxOfAPaymentWhoseAnswerWasLost(t *testing.T) {
	client := answeringEngine(t, func(req request) string {
		return `{"id":"` + req.ID + `","error":{"code":"unavailable","message":"payments: unavailable: broadcast: 504","tx":"tx-lost"}}`
	})
	payment, err := client.Pay(context.Background(), "seed", Sponsorship{Splitter: "TSplitter", Amount: "1"})
	if err == nil || payment.PayTx != "tx-lost" {
		t.Fatalf("payment = %+v, err = %v: the tx the engine sent was dropped", payment, err)
	}
	if errors.Is(err, ErrNoAnswer) {
		t.Fatal("the engine answered, yet the payment reads as unanswered")
	}
}

func TestPayRefusedBeforeSendingHasNoTx(t *testing.T) {
	client := answeringEngine(t, func(req request) string {
		return `{"id":"` + req.ID + `","error":{"code":"insufficient_balance","message":"wallet: insufficient balance"}}`
	})
	payment, err := client.Pay(context.Background(), "seed", Sponsorship{Splitter: "TSplitter", Amount: "1"})
	if err == nil || payment.PayTx != "" || errors.Is(err, ErrNoAnswer) {
		t.Fatalf("payment = %+v, err = %v, want a plain refusal", payment, err)
	}
}

func TestPayToAnEngineThatDiesHasNoAnswer(t *testing.T) {
	client := answeringEngine(t, func(request) string { return "" })
	payment, err := client.Pay(context.Background(), "seed", Sponsorship{Splitter: "TSplitter", Amount: "1"})
	if !errors.Is(err, ErrNoAnswer) || payment.PayTx != "" {
		t.Fatalf("payment = %+v, err = %v: a payment the engine never answered may have been sent", payment, err)
	}
}

func TestIsAvailable(t *testing.T) {
	for _, tt := range []struct {
		name   string
		answer string
		want   bool
	}{
		{"the chain answers", `"result":{"available":true}`, true},
		{"the chain is down", `"result":{"available":false}`, false},
		{"an engine without the method", `"error":{"code":"protocol","message":"ipc: protocol error: unknown method \"available\""}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var asked request
			client := answeringEngine(t, func(req request) string {
				asked = req
				return `{"id":"` + req.ID + `",` + tt.answer + `}`
			})
			if got := client.IsAvailable(context.Background()); got != tt.want {
				t.Fatalf("IsAvailable() = %v, want %v", got, tt.want)
			}
			if asked.Method != "available" || !strings.Contains(string(asked.Params), `"network":"testnet"`) {
				t.Fatalf("asked %s %s, want the availability of testnet", asked.Method, asked.Params)
			}
		})
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

func TestQuoteParamsCarryEveryFieldTheEngineRequires(t *testing.T) {
	sponsorship := Sponsorship{
		Splitter:      "TBuRiiib6EqsezQMMihnBsq2wrAZscxbDy",
		Author:        "THXiCmfr6D4mqAfd4La9EQ5THCx7WsR143",
		Amount:        "1000000",
		MaxFeePercent: 5,
	}
	params := quoteParams("testnet", "TMFCti1AJ7VYQ6QDetHHZu8AkfzMd3P5R6", sponsorship)
	for _, key := range []string{"network", "payer", "splitter", "author", "amount", "max_fee_percent"} {
		if _, ok := params[key]; !ok {
			t.Fatalf("wallet.quote needs %q, got %v", key, params)
		}
	}
	if len(params) != 6 {
		t.Fatalf("wallet.quote was sent something it does not define: %v", params)
	}
}

func TestVerifyParamsCarryEveryFieldTheEngineRequires(t *testing.T) {
	cfg := DefaultConfig("testnet", "")
	params := verifyParams(cfg, "ab12", Sponsorship{
		Splitter: cfg.Splitter,
		OrderId:  "0000000000000000000000000000000000000000000000000000000000000001",
		Author:   "THXiCmfr6D4mqAfd4La9EQ5THCx7WsR143",
		Amount:   "1500000",
	})
	for _, key := range []string{"chain", "network", "tx_id", "order_id", "expected_author", "min_amount", "allowed_assets"} {
		if _, ok := params[key]; !ok {
			t.Fatalf("verify needs %q, got %v", key, params)
		}
	}
	assets, ok := params["allowed_assets"].([]map[string]any)
	if !ok || len(assets) != 1 {
		t.Fatalf("verify must allow exactly the configured splitter, got %v", params["allowed_assets"])
	}
	if assets[0]["contract"] != "TBuRiiib6EqsezQMMihnBsq2wrAZscxbDy" || assets[0]["token"] != "USDT" || assets[0]["decimals"] != uint8(6) {
		t.Fatalf("verify was pointed at another asset: %v", assets[0])
	}
}

func TestOnlyTestnetHasASplitter(t *testing.T) {
	if DefaultConfig("testnet", "").Splitter == "" {
		t.Fatal("testnet must pay through the Nile USDT splitter")
	}
	if DefaultConfig("warpnet", "").Splitter != "" {
		t.Fatal("mainnet has no deployed splitter yet")
	}
}
