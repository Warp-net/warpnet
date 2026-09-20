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

package wallet

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	paymentengine "github.com/Warp-net/payment-engine-lib"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const (
	requestTimeout = 90 * time.Second
	maxLineSize    = 1 << 20
	tokenSymbol    = "USDT"
	NativeCoin     = "TRX"

	paramNetwork    = "network"
	paramToken      = "token"
	paramSeed       = "seed"
	paramPrivateKey = "private_key"
)

var ErrUnavailable = errors.New("wallet: payment engine unavailable")

type Account struct {
	TokenBalance string
	TRX          string
	Activated    bool
	CreatedAt    int64
}

type Config struct {
	BinaryPath string
	Network    string
	Endpoint   string
	Token      string
	Decimals   uint8
	RPS        float64
}

type Transfer struct {
	Tx        string
	Asset     string
	From      string
	To        string
	Value     string
	Timestamp int64
	Incoming  bool
}

type Sponsorship struct {
	Splitter      string
	OrderId       string
	Author        string
	Amount        string
	MaxFeePercent uint64
}

type Payment struct {
	Payer      string
	Token      string
	FeePercent uint64
	Fee        string
	Total      string
	ResetTx    string
	ApproveTx  string
	PayTx      string
}

type request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type responseError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Rejection bool   `json:"rejection"`
}

func (e *responseError) Error() string { return e.Message }

type response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *responseError  `json:"error,omitempty"`
}

type Client struct {
	cfg      Config
	cacheDir func() (string, error)
	engine   func() ([]byte, error)
	mu       sync.Mutex
	cmd      *exec.Cmd
	stop     context.CancelFunc
	stdin    io.WriteCloser
	pending  map[string]chan response
	seq      uint64
	failure  error
}

func New(cfg Config) *Client {
	return &Client{
		cfg:      cfg,
		cacheDir: os.UserCacheDir,
		engine:   paymentengine.GetPaymentEngine,
		pending:  map[string]chan response{},
	}
}

func DefaultConfig(network, binaryPath string) Config {
	if network != "testnet" {
		return Config{
			BinaryPath: binaryPath,
			Network:    "mainnet",
			Endpoint:   "https://api.trongrid.io",
			Token:      tokenSymbol,
			Decimals:   6,
		}
	}
	return Config{
		BinaryPath: binaryPath,
		Network:    "testnet",
		Endpoint:   "https://nile.trongrid.io",
		Token:      tokenSymbol,
		Decimals:   6,
	}
}

func (c *Client) Token() string   { return c.cfg.Token }
func (c *Client) Decimals() uint8 { return c.cfg.Decimals }
func (c *Client) Network() string { return c.cfg.Network }

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd != nil {
		log.Infof("wallet: stopping payment engine")
	}
	if c.stdin != nil {
		_ = c.stdin.Close()
		c.stdin = nil
	}
	if c.stop != nil {
		c.stop()
		c.stop = nil
	}
	c.cmd = nil
}

func (c *Client) prefix() string {
	if c.cfg.Network == "mainnet" {
		return "-mainnet-"
	}
	return "-testnet-"
}

func (c *Client) ensure() error {
	if c.cmd != nil {
		return nil
	}
	args := c.engineArgs()
	binary, err := c.resolveBinary()
	if err != nil {
		log.Errorf("wallet: payment engine binary: %v", err)
		return err
	}
	log.Infof("wallet: starting payment engine %s network=%s endpoint=%s token=%s", binary, c.cfg.Network, c.cfg.Endpoint, c.cfg.Token)
	ctx, stop := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, args...) //nolint:gosec // the path is our own config or the unpacked embedded engine
	stdin, err := cmd.StdinPipe()
	if err != nil {
		stop()
		log.Errorf("wallet: payment engine stdin: %v", err)
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stop()
		log.Errorf("wallet: payment engine stdout: %v", err)
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		stop()
		log.Errorf("wallet: payment engine stderr: %v", err)
		return err
	}
	if err := cmd.Start(); err != nil {
		stop()
		log.Errorf("wallet: payment engine failed to start: %v", err)
		return err
	}
	log.Infof("wallet: payment engine running pid=%d", cmd.Process.Pid)
	c.cmd = cmd
	c.stop = stop
	c.stdin = stdin
	c.failure = nil
	go c.read(stdout)
	go c.readErrors(stderr)
	return nil
}

func (c *Client) readErrors(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 4<<10), maxLineSize)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			log.Errorf("wallet: payment engine: %s", line)
		}
	}
}

func (c *Client) engineArgs() []string {
	args := []string{"serve", c.prefix() + "endpoint", c.cfg.Endpoint, "-network", c.cfg.Network}
	if c.cfg.RPS > 0 {
		args = append(args, "-rps", strconv.FormatFloat(c.cfg.RPS, 'g', -1, 64))
	}
	return args
}

func (c *Client) resolveBinary() (string, error) {
	if c.cfg.BinaryPath != "" {
		if info, err := os.Lstat(c.cfg.BinaryPath); err == nil && info.Mode().IsRegular() {
			return c.cfg.BinaryPath, nil
		}
	}
	binary, err := c.engine()
	if err != nil {
		return "", fmt.Errorf("%w: no payment engine at %q: %w", ErrUnavailable, c.cfg.BinaryPath, err)
	}

	sum := sha256.Sum256(binary)
	name := "payment-engine-" + hex.EncodeToString(sum[:6])
	base, err := c.cacheDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "warpnet")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	switch cached, err := matchesEmbedded(dir, name, sum); {
	case cached:
		return path, nil
	case err != nil && !os.IsNotExist(err):
		log.Warnf("wallet: the payment engine cached at %s is not the build we embed, unpacking it again: %v", path, err)
	}

	tmp, err := os.CreateTemp(dir, name+".*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(binary); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o700); err != nil { //nolint:gosec // the engine has to stay executable
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	return path, nil
}

func matchesEmbedded(dir, name string, want [sha256.Size]byte) (bool, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return false, err
	}
	defer func() { _ = root.Close() }()
	info, err := root.Lstat(name)
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%w: %s is not a regular file", ErrUnavailable, name)
	}
	file, err := root.Open(name)
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return false, err
	}
	if [sha256.Size]byte(digest.Sum(nil)) != want {
		return false, fmt.Errorf("%w: %s holds different bytes", ErrUnavailable, name)
	}
	return true, nil
}

func (c *Client) read(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLineSize)
	for scanner.Scan() {
		var resp response
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			continue
		}
		c.mu.Lock()
		ch := c.pending[resp.ID]
		delete(c.pending, resp.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- resp
		}
	}
	log.Warnf("wallet: payment engine output closed, process gone")
	c.fail(fmt.Errorf("%w: connection closed", ErrUnavailable))
}

func (c *Client) fail(err error) {
	log.Errorf("wallet: payment engine unavailable: %v", err)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failure = err
	if c.stop != nil {
		c.stop()
		c.stop = nil
	}
	c.cmd = nil
	c.stdin = nil
	for id, ch := range c.pending {
		delete(c.pending, id)
		ch <- response{Error: &responseError{Code: "unavailable", Message: err.Error()}}
	}
}

func (c *Client) call(ctx context.Context, method string, params, out any) error {
	err := c.invoke(ctx, method, params, out)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) invoke(ctx context.Context, method string, params, out any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if err := c.ensure(); err != nil {
		c.mu.Unlock()
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	c.seq++
	id := strconv.FormatUint(c.seq, 10)
	ch := make(chan response, 1)
	c.pending[id] = ch
	line, err := json.Marshal(request{ID: id, Method: method, Params: raw})
	if err == nil {
		_, err = c.stdin.Write(append(line, '\n'))
	}
	c.mu.Unlock()
	if err != nil {
		c.forget(id)
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	timer := time.NewTimer(requestTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		c.forget(id)
		return ctx.Err()
	case <-timer.C:
		c.forget(id)
		return fmt.Errorf("%w: timeout", ErrUnavailable)
	case resp := <-ch:
		if resp.Error != nil {
			return resp.Error
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(resp.Result, out)
	}
}

func (c *Client) forget(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *Client) Address(ctx context.Context, seed string) (string, error) {
	var out struct {
		Address string `json:"address"`
	}
	err := c.call(ctx, "wallet.address", map[string]string{paramSeed: seed}, &out)
	return out.Address, err
}

func (c *Client) Export(ctx context.Context, seed string) (address, privateKey string, err error) {
	var out struct {
		Address    string `json:"address"`
		PrivateKey string `json:"private_key"`
	}
	if err = c.call(ctx, "wallet.export", map[string]string{paramSeed: seed}, &out); err != nil {
		return "", "", err
	}
	log.Warnf("wallet: private key exported for %s on %s", out.Address, c.cfg.Network)
	return out.Address, out.PrivateKey, nil
}

func (c *Client) Balance(ctx context.Context, address string) (Account, error) {
	var out struct {
		TokenBalance string `json:"token_balance"`
		TRX          string `json:"trx"`
		Activated    bool   `json:"activated"`
		CreatedAt    int64  `json:"created_at"`
	}
	if err := c.call(ctx, "wallet.balance", map[string]string{paramNetwork: c.cfg.Network, "address": address, paramToken: c.cfg.Token}, &out); err != nil {
		return Account{}, err
	}
	return Account{TokenBalance: out.TokenBalance, TRX: out.TRX, Activated: out.Activated, CreatedAt: out.CreatedAt}, nil
}

func (c *Client) Transfer(ctx context.Context, seed, asset, to, amount string) (string, error) {
	var out struct {
		Tx string `json:"tx"`
	}
	asset = c.assetOr(asset)
	params := map[string]any{paramNetwork: c.cfg.Network, paramSeed: seed, paramToken: asset, "to": to, "amount": amount}
	if err := c.call(ctx, "wallet.transfer", params, &out); err != nil {
		return "", err
	}
	log.Infof("wallet: sent %s of %s to %s on %s, tx %s", amount, asset, to, c.cfg.Network, out.Tx)
	return out.Tx, nil
}

func (c *Client) assetOr(asset string) string {
	if strings.EqualFold(strings.TrimSpace(asset), NativeCoin) {
		return NativeCoin
	}
	return c.cfg.Token
}

func (c *Client) History(ctx context.Context, address, asset string, limit int) ([]Transfer, error) {
	var out struct {
		Transfers []struct {
			Tx        string `json:"tx"`
			Asset     string `json:"asset"`
			From      string `json:"from"`
			To        string `json:"to"`
			Value     string `json:"value"`
			Timestamp int64  `json:"timestamp"`
			Incoming  bool   `json:"incoming"`
		} `json:"transfers"`
	}
	params := map[string]any{paramNetwork: c.cfg.Network, "address": address, paramToken: c.assetOr(asset), "limit": limit}
	if err := c.call(ctx, "wallet.history", params, &out); err != nil {
		return nil, err
	}
	transfers := make([]Transfer, 0, len(out.Transfers))
	for _, t := range out.Transfers {
		transfers = append(transfers, Transfer(t))
	}
	return transfers, nil
}

// Pay settles sponsored content and nothing else. It sends the money through
// one of the project's splitter contracts, which adds the fee on top of the
// amount, routes that fee to a project wallet and emits the Paid event that
// verification later reads to prove this transaction paid for this order.
//
// Paying a person rather than a sponsorship is Transfer: it moves the whole
// amount to the recipient, takes no fee and leaves no receipt to check. Do not
// reach for Pay to send someone money.
//
// Nothing calls this yet, because nothing in warpnet carries a price.
func (c *Client) Pay(ctx context.Context, seed string, s Sponsorship) (Payment, error) {
	var out struct {
		Payer      string `json:"payer"`
		Token      string `json:"token"`
		FeePercent uint64 `json:"fee_percent"`
		Fee        string `json:"fee"`
		Total      string `json:"total"`
		ResetTx    string `json:"reset_tx"`
		ApproveTx  string `json:"approve_tx"`
		PayTx      string `json:"pay_tx"`
	}
	if err := c.call(ctx, "wallet.pay", payParams(c.cfg.Network, seed, s), &out); err != nil {
		return Payment{}, err
	}
	log.Infof(
		"wallet: sponsored %s with %s of %s on %s, fee %s at %d percent, tx %s",
		s.Author, s.Amount, out.Token, c.cfg.Network, out.Fee, out.FeePercent, out.PayTx,
	)
	return Payment(out), nil
}

func payParams(network, seed string, s Sponsorship) map[string]any {
	params := map[string]any{
		paramNetwork: network,
		paramSeed:    seed,
		"splitter":   s.Splitter,
		"order_id":   s.OrderId,
		"author":     s.Author,
		"amount":     s.Amount,
	}
	if s.MaxFeePercent > 0 {
		params["max_fee_percent"] = s.MaxFeePercent
	}
	return params
}
