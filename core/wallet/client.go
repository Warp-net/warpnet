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
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
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
	engineTimeout  = 75 * time.Second
	maxLineSize    = 1 << 20
	tokenSymbol    = "USDT"
	NativeCoin     = "TRX"
	sunPerTRX      = 1_000_000

	testnetSplitter = "TKjWzAR6EBfsadKsqzRAZgtLLgwH9isCa9"
	mainnetUSDT     = "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
	testnetUSDT     = "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"
	maxFeePercent   = 5
	txNotFound      = "tx_not_found"

	paramNetwork    = "network"
	paramToken      = "token"
	paramSeed       = "seed"
	paramPrivateKey = "private_key"
	paramAmount     = "amount"
	paramDeadline   = "deadline"
	paramSplitter   = "splitter"
	paramOrderId    = "order_id"
)

var tronAddress = regexp.MustCompile(`^T[1-9A-HJ-NP-Za-km-z]{33}$`)

var (
	ErrUnavailable = errors.New("wallet: payment engine unavailable")
	ErrNoAnswer    = errors.New("wallet: the payment engine never answered")
	ErrReverted    = errors.New("wallet: the payment reverted")
	ErrAlreadyPaid = errors.New("wallet: the order is already paid")
)

// engineErrors are the engine's error codes a caller acts on rather than shows.
var engineErrors = map[string]error{
	"reverted":     ErrReverted,
	"already_paid": ErrAlreadyPaid,
}

type Account struct {
	TokenBalance string
	TRX          string
	Staked       string
	Unstaking    []Unstaking
	Activated    bool
	CreatedAt    int64
}

type Unstaking struct {
	Amount      string `json:"amount"`
	AvailableAt int64  `json:"available_at"`
}

func (a Account) IsStakeEnough(minTRX uint64) bool {
	staked, ok := new(big.Int).SetString(a.Staked, 10)
	if !ok {
		return minTRX == 0
	}
	return staked.Cmp(sunOf(minTRX)) >= 0
}

func (a Account) IsLockedEnough(minTRX uint64) bool {
	locked, ok := new(big.Int).SetString(a.Staked, 10)
	if !ok {
		locked = new(big.Int)
	}
	for _, u := range a.Unstaking {
		if amount, ok := new(big.Int).SetString(u.Amount, 10); ok {
			locked.Add(locked, amount)
		}
	}
	return locked.Cmp(sunOf(minTRX)) >= 0
}

func IsAddress(address string) bool {
	return tronAddress.MatchString(address)
}

func sunOf(trx uint64) *big.Int {
	return new(big.Int).Mul(new(big.Int).SetUint64(trx), big.NewInt(sunPerTRX))
}

type Config struct {
	BinaryPath    string
	Network       string
	Endpoint      string
	Token         string
	TokenContract string
	Decimals      uint8
	RPS           float64
	Splitter      string
	// Retired are splitters this node no longer pays through but still accepts
	// payments to, so a buyer on an older node is not left with a payment no
	// author will confirm.
	Retired       []string
	MaxFeePercent uint64
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
	Buyer         string
	Amount        string
	MaxFeePercent uint64
	MaxTotal      string
}

// Receipt is what verification says of a payment: whether it is confirmed, and
// when the block that holds it was produced.
type Receipt struct {
	Confirmed bool
	BlockTime time.Time
}

// PaymentState is what the splitter records of an order: whether it is paid,
// and which transaction paid it once the indexer lists it.
type PaymentState struct {
	Paid bool   `json:"paid"`
	Tx   string `json:"tx"`
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

type Quote struct {
	Token          string      `json:"token"`
	FeePercent     uint64      `json:"fee_percent"`
	Fee            string      `json:"fee"`
	Total          string      `json:"total"`
	Balance        string      `json:"balance"`
	TRX            string      `json:"trx"`
	EnergyPrice    int64       `json:"energy_price"`
	BandwidthPrice int64       `json:"bandwidth_price"`
	NetworkFee     string      `json:"network_fee"`
	Steps          []QuoteStep `json:"steps"`
}

type QuoteStep struct {
	Kind        string `json:"kind"`
	Energy      int64  `json:"energy"`
	Bandwidth   int64  `json:"bandwidth"`
	Burn        string `json:"burn"`
	Approximate bool   `json:"approximate"`
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
	Tx        string `json:"tx,omitempty"`
	cause     error
}

func (e *responseError) Error() string { return e.Message }

func (e *responseError) Unwrap() error {
	if e.cause != nil {
		return e.cause
	}
	return engineErrors[e.Code]
}

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
	// tempDir is the private directory the engine was unpacked into for want of
	// a user cache directory, removed on Close.
	tempDir string
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
			BinaryPath:    binaryPath,
			Network:       "mainnet",
			Endpoint:      "https://api.trongrid.io",
			Token:         tokenSymbol,
			TokenContract: mainnetUSDT,
			Decimals:      6,
			MaxFeePercent: maxFeePercent,
		}
	}
	return Config{
		BinaryPath:    binaryPath,
		Network:       "testnet",
		Endpoint:      "https://nile.trongrid.io",
		Token:         tokenSymbol,
		TokenContract: testnetUSDT,
		Decimals:      6,
		Splitter:      testnetSplitter,
		MaxFeePercent: maxFeePercent,
	}
}

func (c *Client) Token() string         { return c.cfg.Token }
func (c *Client) Decimals() uint8       { return c.cfg.Decimals }
func (c *Client) Network() string       { return c.cfg.Network }
func (c *Client) Splitter() string      { return c.cfg.Splitter }
func (c *Client) MaxFeePercent() uint64 { return c.cfg.MaxFeePercent }

// Splitters are every splitter this node accepts payments to: the one it pays
// through first, then the retired ones.
func (c *Client) Splitters() []string {
	out := make([]string, 0, 1+len(c.cfg.Retired))
	for _, s := range append([]string{c.cfg.Splitter}, c.cfg.Retired...) {
		if s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

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
	if c.tempDir != "" {
		_ = os.RemoveAll(c.tempDir)
		c.tempDir = ""
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
	go c.read(cmd, stdout)
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
	args := []string{
		"serve", c.prefix() + "endpoint", c.cfg.Endpoint, "-network", c.cfg.Network,
		"-timeout", engineTimeout.String(),
	}
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
	dir, err := c.engineDir()
	if err != nil {
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

// engineDir is where the embedded engine is unpacked: the user's cache, else a
// fresh directory only this user can write, made once per client. A fixed name
// under the shared temp directory could be made first by another user of the
// machine, who could then swap the engine between its check and its start.
func (c *Client) engineDir() (string, error) {
	if base, err := c.cacheDir(); err == nil {
		dir := filepath.Join(base, "warpnet")
		return dir, os.MkdirAll(dir, 0o700)
	}
	if c.tempDir == "" {
		dir, err := os.MkdirTemp("", "warpnet-")
		if err != nil {
			return "", err
		}
		c.tempDir = dir
	}
	return c.tempDir, nil
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

func (c *Client) read(cmd *exec.Cmd, r io.Reader) {
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
	c.fail(cmd, fmt.Errorf("%w: connection closed", ErrUnavailable))
}

func (c *Client) fail(cmd *exec.Cmd, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd == cmd {
		log.Warnf("wallet: payment engine output closed, process gone")
		log.Errorln(err)
		c.failure = err
		if c.stop != nil {
			c.stop()
			c.stop = nil
		}
		c.cmd = nil
		c.stdin = nil
	}
	for id, ch := range c.pending {
		delete(c.pending, id)
		ch <- response{Error: &responseError{Code: "unavailable", Message: err.Error(), cause: ErrNoAnswer}}
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
		return fmt.Errorf("%w: %w: timeout", ErrUnavailable, ErrNoAnswer)
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
		TokenBalance string      `json:"token_balance"`
		TRX          string      `json:"trx"`
		Staked       string      `json:"staked"`
		Unstaking    []Unstaking `json:"unstaking"`
		Activated    bool        `json:"activated"`
		CreatedAt    int64       `json:"created_at"`
	}
	if err := c.call(ctx, "wallet.balance", map[string]string{paramNetwork: c.cfg.Network, "address": address, paramToken: c.cfg.Token}, &out); err != nil {
		return Account{}, err
	}
	return Account{
		TokenBalance: out.TokenBalance,
		TRX:          out.TRX,
		Staked:       out.Staked,
		Unstaking:    out.Unstaking,
		Activated:    out.Activated,
		CreatedAt:    out.CreatedAt,
	}, nil
}

func (c *Client) Stake(ctx context.Context, seed, amount string) (string, error) {
	return c.sendStaking(ctx, "wallet.stake", seed, amount)
}

func (c *Client) Unstake(ctx context.Context, seed, amount string) (string, error) {
	return c.sendStaking(ctx, "wallet.unstake", seed, amount)
}

func (c *Client) Withdraw(ctx context.Context, seed string) (string, error) {
	return c.sendStaking(ctx, "wallet.withdraw", seed, "")
}

func (c *Client) sendStaking(ctx context.Context, method, seed, amount string) (string, error) {
	var out struct {
		Tx string `json:"tx"`
	}
	params := map[string]any{paramNetwork: c.cfg.Network, paramSeed: seed}
	if amount != "" {
		params[paramAmount] = amount
	}
	if err := c.call(ctx, method, params, &out); err != nil {
		return "", err
	}
	if amount == "" {
		log.Infof("wallet: %s on %s, tx %s", method, c.cfg.Network, out.Tx)
	} else {
		log.Infof("wallet: %s %s sun on %s, tx %s", method, amount, c.cfg.Network, out.Tx)
	}
	return out.Tx, nil
}

func (c *Client) Transfer(ctx context.Context, seed, asset, to, amount string) (string, error) {
	var out struct {
		Tx string `json:"tx"`
	}
	asset = c.assetOr(asset)
	params := map[string]any{paramNetwork: c.cfg.Network, paramSeed: seed, paramToken: asset, "to": to, paramAmount: amount, paramDeadline: deadline()}
	if err := c.call(ctx, "wallet.transfer", params, &out); err != nil {
		// A transfer whose answer was lost may still be on chain; its id is what
		// keeps the owner from sending the same money twice.
		var engineErr *responseError
		if errors.As(err, &engineErr) && engineErr.Tx != "" {
			return engineErr.Tx, err
		}
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
		var engineErr *responseError
		if errors.As(err, &engineErr) && engineErr.Tx != "" {
			return Payment{PayTx: engineErr.Tx}, err
		}
		return Payment{}, err
	}
	log.Infof(
		"wallet: sponsored %s with %s of %s on %s, fee %s at %d percent, tx %s",
		s.Author, s.Amount, out.Token, c.cfg.Network, out.Fee, out.FeePercent, out.PayTx,
	)
	return Payment(out), nil
}

// Quote asks what Pay would cost the wallet of seed for s, without signing
// anything: the token amounts exactly and the TRX the network would burn.
func (c *Client) Quote(ctx context.Context, seed string, s Sponsorship) (Quote, error) {
	payer, err := c.Address(ctx, seed)
	if err != nil {
		return Quote{}, err
	}
	var out Quote
	err = c.call(ctx, "wallet.quote", quoteParams(c.cfg.Network, payer, s), &out)
	return out, err
}

func (c *Client) IsAvailable(ctx context.Context) bool {
	var out struct {
		Available bool `json:"available"`
	}
	err := c.call(ctx, "available", map[string]string{"network": c.cfg.Network}, &out)
	return err == nil && out.Available
}

// Payment asks the splitter whether the buyer's order is paid and which of the
// buyer's transactions from since onwards paid it, so a payment whose answer was
// lost is found rather than sent again.
func (c *Client) Payment(ctx context.Context, s Sponsorship, since time.Time) (PaymentState, error) {
	var out PaymentState
	params := map[string]any{
		paramNetwork:  c.cfg.Network,
		paramSplitter: s.Splitter,
		"buyer":       s.Buyer,
		paramOrderId:  s.OrderId,
		"since":       since.UnixMilli(),
	}
	err := c.call(ctx, "wallet.payment", params, &out)
	return out, err
}

func (c *Client) IsPaid(ctx context.Context, txId string, s Sponsorship) (Receipt, error) {
	var out struct {
		Confirmed     bool   `json:"confirmed"`
		Buyer         string `json:"buyer"`
		AuthorAmount  string `json:"author_amount"`
		FeeAmount     string `json:"fee_amount"`
		Confirmations uint64 `json:"confirmations"`
		BlockTime     int64  `json:"block_time"`
	}
	err := c.call(ctx, "verify", verifyParams(c.cfg, txId, s), &out)
	var engineErr *responseError
	if errors.As(err, &engineErr) && engineErr.Code == txNotFound {
		return Receipt{}, nil
	}
	if err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{Confirmed: out.Confirmed}
	if out.BlockTime > 0 {
		receipt.BlockTime = time.UnixMilli(out.BlockTime)
	}
	if out.Confirmed {
		log.Infof(
			"wallet: %s paid %s of %s to %s on %s, fee %s, %d confirmations, tx %s",
			out.Buyer, out.AuthorAmount, c.cfg.Token, s.Author, c.cfg.Network, out.FeeAmount, out.Confirmations, txId,
		)
	}
	return receipt, nil
}

func quoteParams(network, payer string, s Sponsorship) map[string]any {
	params := map[string]any{
		paramNetwork:  network,
		"payer":       payer,
		paramSplitter: s.Splitter,
		"author":      s.Author,
		paramAmount:   s.Amount,
	}
	if s.MaxFeePercent > 0 {
		params["max_fee_percent"] = s.MaxFeePercent
	}
	return params
}

func verifyParams(cfg Config, txId string, s Sponsorship) map[string]any {
	splitters := (&Client{cfg: cfg}).Splitters()
	if len(splitters) == 0 {
		splitters = []string{s.Splitter}
	}
	assets := make([]map[string]any, 0, len(splitters))
	for _, splitter := range splitters {
		asset := map[string]any{paramToken: cfg.Token, "contract": splitter, "decimals": cfg.Decimals}
		if cfg.TokenContract != "" {
			asset["token_contract"] = cfg.TokenContract
		}
		assets = append(assets, asset)
	}
	params := map[string]any{
		"chain":           "tron",
		paramNetwork:      cfg.Network,
		"tx_id":           txId,
		paramOrderId:      s.OrderId,
		"expected_author": s.Author,
		"min_amount":      s.Amount,
		"allowed_assets":  assets,
	}
	if s.Buyer != "" {
		params["expected_buyer"] = s.Buyer
	}
	return params
}

func payParams(network, seed string, s Sponsorship) map[string]any {
	params := map[string]any{
		paramNetwork:  network,
		paramSeed:     seed,
		paramSplitter: s.Splitter,
		paramOrderId:  s.OrderId,
		"author":      s.Author,
		paramAmount:   s.Amount,
		paramDeadline: deadline(),
	}
	if s.MaxFeePercent > 0 {
		params["max_fee_percent"] = s.MaxFeePercent
	}
	if s.MaxTotal != "" {
		params["max_total"] = s.MaxTotal
	}
	return params
}

// deadline is when this node stops waiting for the engine: after it the engine
// sends nothing, so a payment cannot go out once its caller has given up on it.
func deadline() int64 {
	return time.Now().Add(engineTimeout).UnixMilli()
}
