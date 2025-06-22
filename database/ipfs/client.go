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

package ipfs

import (
	"context"
	"errors"
	"fmt"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/ipfs/boxo/files"
	"github.com/ipfs/boxo/path"
	"github.com/ipfs/go-cid"
	golog "github.com/ipfs/go-log/v2"
	"github.com/ipfs/kubo/core/coreiface/options"
	kubop2p "github.com/ipfs/kubo/core/node/libp2p"
	"github.com/ipfs/kubo/plugin/loader"
	"github.com/klauspost/compress/zstd"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/multiformats/go-multihash"
	log "github.com/sirupsen/logrus"
	"io"
	"strings"

	"os"

	_ "github.com/ipfs/go-ds-flatfs"

	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core"
	"github.com/ipfs/kubo/core/coreapi"
	coreiface "github.com/ipfs/kubo/core/coreiface"
	"github.com/ipfs/kubo/repo/fsrepo"
	_ "google.golang.org/genproto/googleapis/api/annotations"
)

const (
	repoPath = "/tmp/ipfs" // TODO
)

type WarpNodeIdentifier interface {
	NodeInfo() warpnet.NodeInfo
	PrivateKey() warpnet.WarpPrivateKey
}

type Client struct {
	api  coreiface.CoreAPI
	node *core.IpfsNode
}

func NewIPFS(ctx context.Context, n host.Host) (*Client, error) {
	_ = golog.SetLogLevel("core", "error")

	plugins, err := loader.NewPluginLoader(repoPath)
	if err != nil {
		return nil, fmt.Errorf("error loading plugins: %s", err)
	}

	if err := plugins.Initialize(); err != nil {
		return nil, fmt.Errorf("error initializing plugins: %s", err)
	}

	if err := plugins.Inject(); err != nil {
		return nil, fmt.Errorf("error initializing plugins: %s", err)
	}

	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		err = os.MkdirAll(repoPath, 0o700)
		if err != nil {
			return nil, fmt.Errorf("failed to create repo dir: %w", err)
		}
	}

	cfg, err := config.Init(os.Stdout, 2048)
	if err != nil {
		return nil, fmt.Errorf("config init failed: %w", err)
	}

	addrs := make([]string, 0, len(n.Addrs()))
	for _, a := range n.Addrs() {
		addrs = append(addrs, a.String())
	}
	fmt.Println(strings.Join(addrs, ","), "???????????????????")
	// Disable everything that talks to the outside world
	cfg.Bootstrap = []string{}
	cfg.Addresses.API = []string{}
	cfg.Addresses.Gateway = []string{}
	cfg.Addresses.Swarm = addrs
	cfg.AutoTLS.Enabled = config.False
	cfg.AutoTLS.AutoWSS = config.False
	cfg.Routing.Type = config.NewOptionalString("none")
	cfg.Discovery.MDNS.Enabled = false
	cfg.Datastore = config.Datastore{
		StorageMax:         "10GB",
		StorageGCWatermark: 90,
		GCPeriod:           "1h",
		HashOnRead:         false,
		Spec:               defaultDatastoreSpec(),
	}

	if !fsrepo.IsInitialized(repoPath) {
		if err := fsrepo.Init(repoPath, cfg); err != nil {
			return nil, fmt.Errorf("repo init failed: %w", err)
		}
	}
	repo, err := fsrepo.Open(repoPath)
	if err != nil {
		return nil, fmt.Errorf("repo open failed: %w", err)
	}
	if err := repo.SetConfig(cfg); err != nil {
		return nil, fmt.Errorf("repo init failed: %w", err)
	}

	buildCfg := &core.BuildCfg{
		Online:    true,
		Permanent: true,
		Routing:   kubop2p.NilRouterOption,
		Repo:      repo,
		Host: func(_ peer.ID, _ peerstore.Peerstore, _ ...libp2p.Option) (host.Host, error) {
			return n, nil
		},
	}
	node, err := core.NewNode(ctx, buildCfg)
	if err != nil {
		return nil, fmt.Errorf("node create failed: %w", err)
	}

	api, err := coreapi.NewCoreAPI(node)
	if err != nil {
		return nil, fmt.Errorf("coreapi init failed: %w", err)
	}

	return &Client{
		api:  api,
		node: node,
	}, nil
}

func defaultDatastoreSpec() map[string]interface{} {
	return map[string]interface{}{
		"type": "mount",
		"mounts": []interface{}{
			map[string]interface{}{
				"mountpoint": "/blocks",
				"type":       "measure",
				"prefix":     "flatfs.datastore",
				"child": map[string]interface{}{
					"type":      "flatfs",
					"path":      "blocks",
					"sync":      true,
					"shardFunc": "/repo/flatfs/shard/v1/next-to-last/2",
				},
			},
			map[string]interface{}{
				"mountpoint": "/",
				"type":       "measure",
				"prefix":     "leveldb.datastore",
				"child": map[string]interface{}{
					"type": "levelds",
					"path": "datastore",
				},
			},
		},
	}
}

func (c *Client) ID() string {
	return c.node.Identity.String()
}

func (c *Client) PutStream(ctx context.Context, reader io.ReadCloser) (id string, _ error) {
	if reader == nil {
		return cid.Undef.String(), fmt.Errorf("nil reader")
	}

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		zw, err := zstd.NewWriter(
			pw,
			zstd.WithEncoderConcurrency(1),
			zstd.WithEncoderLevel(zstd.SpeedBestCompression),
			zstd.WithWindowSize(128<<20),
			zstd.WithZeroFrames(false),
			zstd.WithLowerEncoderMem(false),
		)
		if err != nil {
			err := fmt.Errorf("failed to init zstd writer: %w", err)
			log.Error(err)
			_ = pw.CloseWithError(err)
			return
		}
		defer zw.Close()

		_, err = io.Copy(zw, reader)
		if err != nil {
			err := fmt.Errorf("compression write: %w", err)
			log.Error(err)
			_ = pw.CloseWithError(err)
			return
		}
	}()

	f := files.NewReaderFile(pr)

	immPath, err := c.api.Unixfs().Add(
		ctx,
		f,
		options.Unixfs.CidVersion(1),
		options.Unixfs.Hash(multihash.SHA2_256),
		options.Unixfs.RawLeaves(false),
		options.Unixfs.Inline(false),
	)
	if err != nil {
		return cid.Undef.String(), fmt.Errorf("failed to add to ipfs: %w", err)
	}

	if err := c.api.Pin().Add(ctx, immPath); err != nil {
		log.Errorf("failed to pin CID %s: %v", immPath.RootCid(), err)
		return immPath.RootCid().String(), nil
	}

	return immPath.RootCid().String(), nil
}

type compressedReader struct {
	io.Reader
	closers []io.Closer
}

type errorCloser struct {
	silentCloserFunc func()
}

func (c *errorCloser) Close() error {
	c.silentCloserFunc()
	return nil
}

func (w *compressedReader) Close() error {
	var errs []error
	for _, c := range w.closers {
		if err := c.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *Client) GetStream(ctx context.Context, id string) (io.ReadCloser, error) {
	cID, err := cid.Decode(id)
	if err != nil {
		return nil, err
	}
	immPath := path.FromCid(cID)
	nodeFile, err := c.api.Unixfs().Get(ctx, immPath)
	if err != nil {
		return nil, fmt.Errorf("ipfs get failed: %w", err)
	}

	f, ok := nodeFile.(files.File)
	if !ok {
		_ = nodeFile.Close()
		return nil, io.ErrUnexpectedEOF
	}

	if err := c.api.Pin().Add(ctx, immPath); err != nil {
		log.Errorf("failed to pin CID %s: %v", immPath.RootCid(), err)
	}

	decoder, err := zstd.NewReader(f)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("zstd decompression failed: %w", err)
	}

	return &compressedReader{
		Reader: decoder,
		closers: []io.Closer{
			&errorCloser{decoder.Close},
			f,
		},
	}, nil
}

func (c *Client) Close() error {
	return c.node.Close()
}
