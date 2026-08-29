//go:build !windows

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

package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/json"
)

const (
	// latestReleaseAPI resolves to the newest published Warpnet release.
	latestReleaseAPI = "https://api.github.com/repos/Warp-net/warpnet/releases/latest"

	downloadTimeout = 10 * time.Minute

	maxMetadataSize = 1 << 20   // release JSON and checksum listings
	maxArchiveSize  = 128 << 20 // compressed release asset
)

// Release is a published release with the assets attached to it.
type Release struct {
	Version *semver.Version
	assets  map[string]string // asset name -> download URL
}

// AssetURL returns the download URL of the named asset.
func (r Release) AssetURL(name string) (string, error) {
	url, ok := r.assets[name]
	if !ok {
		return "", fmt.Errorf("selfupdate: %w: %s", ErrAssetNotFound, name)
	}
	return url, nil
}

// githubReleases reads releases and their assets from the GitHub API.
type githubReleases struct {
	ctx       context.Context
	client    *http.Client
	apiURL    string
	userAgent string
}

func newGitHubReleases(ctx context.Context, current *semver.Version) *githubReleases {
	userAgent := warpnet.WarpnetName
	if current != nil {
		userAgent += "/" + current.String()
	}
	return &githubReleases{
		ctx:       ctx,
		client:    &http.Client{Timeout: downloadTimeout},
		apiURL:    latestReleaseAPI,
		userAgent: userAgent,
	}
}

func (g *githubReleases) Latest() (Release, error) {
	body, err := g.Read(g.apiURL)
	if err != nil {
		return Release{}, err
	}

	var payload struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Release{}, fmt.Errorf("selfupdate: decoding release: %w", err)
	}
	if strings.TrimSpace(payload.TagName) == "" {
		return Release{}, fmt.Errorf("selfupdate: %w: %s", ErrNoReleaseTag, g.apiURL)
	}

	version, err := semver.NewVersion(strings.TrimSpace(payload.TagName))
	if err != nil {
		return Release{}, fmt.Errorf("selfupdate: parsing release tag %s: %w", payload.TagName, err)
	}

	assets := make(map[string]string, len(payload.Assets))
	for _, a := range payload.Assets {
		assets[a.Name] = a.URL
	}
	return Release{Version: version, assets: assets}, nil
}

func (g *githubReleases) Read(url string) ([]byte, error) {
	resp, err := g.get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataSize))
	if err != nil {
		return nil, fmt.Errorf("selfupdate: reading %s: %w", url, err)
	}
	if int64(len(data)) == maxMetadataSize {
		return nil, fmt.Errorf("selfupdate: %w: %s", ErrTooLarge, url)
	}
	return data, nil
}

func (g *githubReleases) Download(url, dstPath string) (_ string, err error) {
	resp, err := g.get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	dst, err := os.Create(dstPath) //nolint:gosec // path sits next to the running executable
	if err != nil {
		return "", fmt.Errorf("selfupdate: creating %s: %w", dstPath, err)
	}
	defer func() {
		if closeErr := dst.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("selfupdate: closing %s: %w", dstPath, closeErr)
		}
	}()

	h := sha256.New()
	written, err := io.Copy(io.MultiWriter(dst, h), io.LimitReader(resp.Body, maxArchiveSize))
	if err != nil {
		return "", fmt.Errorf("selfupdate: downloading %s: %w", url, err)
	}
	if written == maxArchiveSize {
		return "", fmt.Errorf("selfupdate: %w: %s", ErrTooLarge, url)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (g *githubReleases) get(url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(g.ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: building request %s: %w", url, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", g.userAgent)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("selfupdate: requesting %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("selfupdate: %w: %s: %s", ErrUnexpectedStatus, url, resp.Status)
	}
	return resp, nil
}
