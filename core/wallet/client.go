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
	"sync"
	"time"

	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

const (
	requestTimeout = 90 * time.Second
	maxLineSize    = 1 << 20
)

var ErrUnavailable = errors.New("wallet: payment engine unavailable")

var sensitiveParams = map[string]bool{"seed": true, "private_key": true}

func safeParams(params any) string {
	raw, err := json.Marshal(params)
	if err != nil {
		return "{}"
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return "{}"
	}
	for name := range fields {
		if sensitiveParams[name] {
			delete(fields, name)
		}
	}
	out, err := json.Marshal(fields)
	if err != nil {
		return "{}"
	}
	return string(out)
}

type Account struct {
	TokenBalance string
	TRX          string
	Activated    bool
	CreatedAt    int64
}

type Config struct {
	BinaryPath  string
	Network     string
	Endpoint    string
	Registry    string
	APIKey      string
	Token       string
	Decimals    uint8
	RPS         float64
	BinaryBytes []byte
}

type Transfer struct {
	Tx        string
	From      string
	To        string
	Value     string
	Timestamp int64
	Incoming  bool
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
	cfg     Config
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	pending map[string]chan response
	seq     uint64
	failure error
}

func New(cfg Config) *Client {
	return &Client{cfg: cfg, pending: map[string]chan response{}}
}

func DefaultConfig(network, binaryPath string) Config {
	if network != "testnet" {
		return Config{
			BinaryPath: binaryPath,
			Network:    "mainnet",
			Endpoint:   "https://api.trongrid.io",
			Token:      "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
			Decimals:   6,
		}
	}
	return Config{
		BinaryPath: binaryPath,
		Network:    "testnet",
		Endpoint:   "https://nile.trongrid.io",
		Registry:   "TG2C8vi4gFqVvB8wHcodLbLekJ5VTweZAT",
		Token:      "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
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
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		c.cmd = nil
	}
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
	args := []string{"serve", c.prefix() + "registry", c.cfg.Registry, c.prefix() + "endpoint", c.cfg.Endpoint, "-network", c.cfg.Network}
	if c.cfg.APIKey != "" {
		args = append(args, "-api-key", c.cfg.APIKey)
	}
	if c.cfg.RPS > 0 {
		args = append(args, "-rps", strconv.FormatFloat(c.cfg.RPS, 'g', -1, 64))
	}
	binary, err := c.resolveBinary()
	if err != nil {
		log.Errorf("wallet: payment engine binary: %v", err)
		return err
	}
	log.Infof("wallet: starting payment engine %q network=%s endpoint=%s registry=%s", binary, c.cfg.Network, c.cfg.Endpoint, c.cfg.Registry)
	cmd := exec.Command(binary, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Errorf("wallet: payment engine stdin: %v", err)
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Errorf("wallet: payment engine stdout: %v", err)
		return err
	}
	if err := cmd.Start(); err != nil {
		log.Errorf("wallet: payment engine failed to start: %v", err)
		return err
	}
	log.Infof("wallet: payment engine running pid=%d", cmd.Process.Pid)
	c.cmd = cmd
	c.stdin = stdin
	c.failure = nil
	go c.read(stdout)
	return nil
}

func (c *Client) resolveBinary() (string, error) {
	if c.cfg.BinaryPath != "" {
		if _, err := os.Stat(c.cfg.BinaryPath); err == nil {
			return c.cfg.BinaryPath, nil
		}
	}
	if len(c.cfg.BinaryBytes) == 0 {
		return "", fmt.Errorf("%w: no payment engine at %q and none embedded", ErrUnavailable, c.cfg.BinaryPath)
	}

	sum := sha256.Sum256(c.cfg.BinaryBytes)
	name := "payment-engine-" + hex.EncodeToString(sum[:6])
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "warpnet")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if info, err := os.Stat(path); err == nil && info.Size() == int64(len(c.cfg.BinaryBytes)) {
		return path, nil
	}

	tmp, err := os.CreateTemp(dir, name+".*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(c.cfg.BinaryBytes); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o700); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	log.Infof("wallet: unpacked the embedded payment engine to %q", path)
	return path, nil
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
	c.cmd = nil
	c.stdin = nil
	for id, ch := range c.pending {
		delete(c.pending, id)
		ch <- response{Error: &responseError{Code: "unavailable", Message: err.Error()}}
	}
}

func (c *Client) call(ctx context.Context, method string, params, out any) error {
	started := time.Now()
	log.Infof("wallet: engine request %s %s", method, safeParams(params))
	err := c.invoke(ctx, method, params, out)
	elapsed := time.Since(started).Round(time.Millisecond)
	if err != nil {
		log.Errorf("wallet: engine request %s failed in %s: %v", method, elapsed, err)
		return err
	}
	log.Infof("wallet: engine request %s succeeded in %s", method, elapsed)
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
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
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
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
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
	err := c.call(ctx, "wallet.address", map[string]string{"seed": seed}, &out)
	return out.Address, err
}

func (c *Client) Export(ctx context.Context, seed string) (address, privateKey string, err error) {
	var out struct {
		Address    string `json:"address"`
		PrivateKey string `json:"private_key"`
	}
	if err = c.call(ctx, "wallet.export", map[string]string{"seed": seed}, &out); err != nil {
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
	if err := c.call(ctx, "wallet.balance", map[string]string{"network": c.cfg.Network, "address": address, "token": c.cfg.Token}, &out); err != nil {
		return Account{}, err
	}
	return Account{TokenBalance: out.TokenBalance, TRX: out.TRX, Activated: out.Activated, CreatedAt: out.CreatedAt}, nil
}

func (c *Client) Transfer(ctx context.Context, seed, to, amount string) (string, error) {
	var out struct {
		Tx string `json:"tx"`
	}
	params := map[string]any{"network": c.cfg.Network, "seed": seed, "token": c.cfg.Token, "to": to, "amount": amount}
	if err := c.call(ctx, "wallet.transfer", params, &out); err != nil {
		return "", err
	}
	log.Infof("wallet: sent %s of %s to %s on %s, tx %s", amount, c.cfg.Token, to, c.cfg.Network, out.Tx)
	return out.Tx, nil
}

func (c *Client) History(ctx context.Context, address string, limit int) ([]Transfer, error) {
	var out struct {
		Transfers []struct {
			Tx        string `json:"tx"`
			From      string `json:"from"`
			To        string `json:"to"`
			Value     string `json:"value"`
			Timestamp int64  `json:"timestamp"`
			Incoming  bool   `json:"incoming"`
		} `json:"transfers"`
	}
	params := map[string]any{"network": c.cfg.Network, "address": address, "token": c.cfg.Token, "limit": limit}
	if err := c.call(ctx, "wallet.history", params, &out); err != nil {
		return nil, err
	}
	transfers := make([]Transfer, 0, len(out.Transfers))
	for _, t := range out.Transfers {
		transfers = append(transfers, Transfer(t))
	}
	return transfers, nil
}
