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
	"time"

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

func TestResolveBinaryWithoutACacheDirUnpacksWhereNoOneElseCanWrite(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	shared := filepath.Join(os.TempDir(), "warpnet")
	if err := os.MkdirAll(shared, 0o777); err != nil { //nolint:gosec // another user of the machine made it first
		t.Fatal(err)
	}
	client, _ := engineIn(t, Config{Network: "testnet"}, []byte("embedded engine bytes"))
	client.cacheDir = func() (string, error) { return "", errors.New("no home") }

	path, err := client.resolveBinary()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(path)
	if dir == shared {
		t.Fatal("unpacked into a directory another user could have made")
	}
	if info, err := os.Stat(dir); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		t.Fatalf("dir %s: %v, %v", dir, info, err)
	}
	if again, err := client.resolveBinary(); err != nil || again != path {
		t.Fatalf("unpacked again into %s: %v", again, err)
	}
	client.Close()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("%s was left behind: %v", dir, err)
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
	for _, key := range []string{"splitter", "order_id", "author", "amount", "network", "seed", "max_fee_percent", "deadline"} {
		if _, ok := params[key]; !ok {
			t.Fatalf("wallet.pay needs %q, got %v", key, params)
		}
	}
	if len(params) != 8 {
		t.Fatalf("wallet.pay was sent something it does not define: %v", params)
	}
	deadline := time.UnixMilli(params["deadline"].(int64))
	if until := time.Until(deadline); until <= 0 || until > requestTimeout {
		t.Fatalf("deadline in %v: the engine must give up before this node does", until)
	}

	sponsorship.MaxTotal = "1050000"
	if got := payParams("testnet", "abcdef", sponsorship)["max_total"]; got != "1050000" {
		t.Fatalf("max_total = %v: the most the buyer agreed to pay must reach the engine", got)
	}

	sponsorship.MaxFeePercent, sponsorship.MaxTotal = 0, ""
	params = payParams("testnet", "abcdef", sponsorship)
	if _, ok := params["max_fee_percent"]; ok {
		t.Fatal("a sponsorship that agreed no ceiling must not send one")
	}
	if _, ok := params["max_total"]; ok {
		t.Fatal("a sponsorship that agreed no total must not send one")
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
	if assets[0]["contract"] != "TKjWzAR6EBfsadKsqzRAZgtLLgwH9isCa9" || assets[0]["token"] != "USDT" || assets[0]["decimals"] != uint8(6) {
		t.Fatalf("verify was pointed at another asset: %v", assets[0])
	}
	if assets[0]["token_contract"] != "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf" {
		t.Fatalf("token_contract = %v: a splitter of another token must not pass for USDT", assets[0]["token_contract"])
	}
}

func TestVerifyAcceptsPaymentsToRetiredSplitters(t *testing.T) {
	cfg := DefaultConfig("testnet", "")
	cfg.Retired = []string{"TBuRiiib6EqsezQMMihnBsq2wrAZscxbDy", cfg.Splitter}
	assets := verifyParams(cfg, "ab12", Sponsorship{Splitter: cfg.Splitter, OrderId: "01", Author: "THXiCmfr6D4mqAfd4La9EQ5THCx7WsR143", Amount: "1"})["allowed_assets"].([]map[string]any)
	if len(assets) != 2 || assets[0]["contract"] != cfg.Splitter || assets[1]["contract"] != "TBuRiiib6EqsezQMMihnBsq2wrAZscxbDy" {
		t.Fatalf("assets = %v: a buyer on an older node still pays through the retired splitter", assets)
	}
}

func TestVerifyParamsNameTheBuyerOnlyWhenKnown(t *testing.T) {
	cfg := DefaultConfig("testnet", "")
	s := Sponsorship{Splitter: cfg.Splitter, OrderId: "01", Author: "THXiCmfr6D4mqAfd4La9EQ5THCx7WsR143", Amount: "1500000"}
	if _, ok := verifyParams(cfg, "ab12", s)["expected_buyer"]; ok {
		t.Fatal("verify must not expect a buyer nobody named")
	}
	s.Buyer = "TMFCti1AJ7VYQ6QDetHHZu8AkfzMd3P5R6"
	if got := verifyParams(cfg, "ab12", s)["expected_buyer"]; got != s.Buyer {
		t.Fatalf("expected_buyer = %v, want %s", got, s.Buyer)
	}
}

func TestIsStakeEnoughComparesWholeTRXWithSun(t *testing.T) {
	cases := []struct {
		staked string
		min    uint64
		want   bool
	}{
		{"", 0, true},
		{"", 1, false},
		{"0", 0, true},
		{"499999999", 500, false},
		{"500000000", 500, true},
		{"18446744073709551615000000", 18446744073709551615, true},
		{"garbage", 1, false},
	}
	for _, tc := range cases {
		if got := (Account{Staked: tc.staked}).IsStakeEnough(tc.min); got != tc.want {
			t.Fatalf("staked %q, min %d TRX: got %v, want %v", tc.staked, tc.min, got, tc.want)
		}
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

func TestStakingReachesTheEngine(t *testing.T) {
	var asked []request
	client := answeringEngine(t, func(req request) string {
		asked = append(asked, req)
		return `{"id":"` + req.ID + `","result":{"owner":"TOwner","tx":"tx-` + req.Method + `"}}`
	})
	ctx := context.Background()
	for _, tt := range []struct {
		method string
		send   func() (string, error)
		amount bool
	}{
		{"wallet.stake", func() (string, error) { return client.Stake(ctx, "seed", "5000000") }, true},
		{"wallet.unstake", func() (string, error) { return client.Unstake(ctx, "seed", "2000000") }, true},
		{"wallet.withdraw", func() (string, error) { return client.Withdraw(ctx, "seed") }, false},
	} {
		tx, err := tt.send()
		if err != nil || tx != "tx-"+tt.method {
			t.Fatalf("%s: tx = %q, err = %v", tt.method, tx, err)
		}
		last := asked[len(asked)-1]
		if last.Method != tt.method || !strings.Contains(string(last.Params), `"seed":"seed"`) || !strings.Contains(string(last.Params), `"network":"testnet"`) {
			t.Fatalf("asked %s %s", last.Method, last.Params)
		}
		if strings.Contains(string(last.Params), `"amount"`) != tt.amount {
			t.Fatalf("%s params %s: amount sent = %v, want %v", tt.method, last.Params, !tt.amount, tt.amount)
		}
	}
}

func TestBalanceListsWhatIsBeingUnstaked(t *testing.T) {
	client := answeringEngine(t, func(req request) string {
		return `{"id":"` + req.ID + `","result":{"token_balance":"1","trx":"2","staked":"3","unstaking":[{"amount":"4","available_at":1758000000000}]}}`
	})
	account, err := client.Balance(context.Background(), "TAddr")
	if err != nil {
		t.Fatal(err)
	}
	if account.Staked != "3" || len(account.Unstaking) != 1 || account.Unstaking[0] != (Unstaking{Amount: "4", AvailableAt: 1758000000000}) {
		t.Fatalf("account = %+v", account)
	}
}

func TestIsAddressTakesOnlyTheBase58Form(t *testing.T) {
	if !IsAddress("TZEDAzs41eK4ipB9gaUnKbaWLvjyn2paQa") {
		t.Fatal("a base58 address is refused")
	}
	for _, other := range []string{
		"", " TZEDAzs41eK4ipB9gaUnKbaWLvjyn2paQa", "TZEDAzs41eK4ipB9gaUnKbaWLvjyn2paQa\n",
		"41ff21468791a710e549e6fbb3b7b6df5324b893c5", "0x41ff21468791a710e549e6fbb3b7b6df5324b893c5",
		"ff21468791a710e549e6fbb3b7b6df5324b893c5", "TZEDAzs41eK4ipB9gaUnKbaWLvjyn2paQ0", "TBuyerWallet",
	} {
		if IsAddress(other) {
			t.Fatalf("%q passes as a base58 address: another spelling of one wallet would slip past a string compare", other)
		}
	}
}

func TestIsLockedEnoughCountsWhatIsBeingUnstaked(t *testing.T) {
	account := Account{Staked: "100000000", Unstaking: []Unstaking{{Amount: "300000000"}, {Amount: "100000000"}}}
	if account.IsStakeEnough(500) {
		t.Fatal("TRX being unstaked is not a stake a new order may lean on")
	}
	if !account.IsLockedEnough(500) || account.IsLockedEnough(501) {
		t.Fatal("staked and unstaking TRX are both locked")
	}
	if !(Account{}).IsLockedEnough(0) || (Account{}).IsLockedEnough(1) {
		t.Fatal("an empty account locks nothing")
	}
}

func TestPayNamesTheEngineVerdictsACallerActsOn(t *testing.T) {
	for code, want := range map[string]error{"reverted": ErrReverted, "already_paid": ErrAlreadyPaid} {
		client := answeringEngine(t, func(req request) string {
			return `{"id":"` + req.ID + `","error":{"code":"` + code + `","message":"wallet: ` + code + `","tx":"tx-1"}}`
		})
		payment, err := client.Pay(context.Background(), "seed", Sponsorship{Splitter: "TSplitter", Amount: "1"})
		if !errors.Is(err, want) || payment.PayTx != "tx-1" {
			t.Fatalf("%s: payment = %+v, err = %v", code, payment, err)
		}
	}
}

func TestPaymentAsksTheSplitterAboutTheOrder(t *testing.T) {
	var asked request
	client := answeringEngine(t, func(req request) string {
		asked = req
		return `{"id":"` + req.ID + `","result":{"paid":true,"tx":"tx-found"}}`
	})
	since := time.UnixMilli(1789073000000)
	state, err := client.Payment(context.Background(), Sponsorship{Splitter: "TSplitter", Buyer: "TBuyer", OrderId: "ab"}, since)
	if err != nil || !state.Paid || state.Tx != "tx-found" {
		t.Fatalf("state = %+v, err = %v", state, err)
	}
	var params map[string]any
	if err := json.Unmarshal(asked.Params, &params); err != nil {
		t.Fatal(err)
	}
	if asked.Method != "wallet.payment" || params["splitter"] != "TSplitter" || params["buyer"] != "TBuyer" || params["order_id"] != "ab" || params["since"] != float64(1789073000000) {
		t.Fatalf("request = %s %v", asked.Method, params)
	}
}

func TestIsPaidReportsWhenThePaymentWasMade(t *testing.T) {
	client := answeringEngine(t, func(req request) string {
		return `{"id":"` + req.ID + `","result":{"confirmed":true,"block_time":1788984597000}}`
	})
	receipt, err := client.IsPaid(context.Background(), "tx", Sponsorship{Splitter: "TSplitter"})
	if err != nil || !receipt.Confirmed || !receipt.BlockTime.Equal(time.UnixMilli(1788984597000)) {
		t.Fatalf("receipt = %+v, err = %v", receipt, err)
	}
}

func TestTransferKeepsTheTxOfATransferWhoseAnswerWasLost(t *testing.T) {
	client := answeringEngine(t, func(req request) string {
		return `{"id":"` + req.ID + `","error":{"code":"unavailable","message":"payments: unavailable: broadcast: 504","tx":"tx-maybe"}}`
	})
	tx, err := client.Transfer(context.Background(), "seed", "TRX", "TRecipient", "5")
	if err == nil || tx != "tx-maybe" {
		t.Fatalf("tx = %q, err = %v: the owner must learn the transfer may be on chain", tx, err)
	}
}
