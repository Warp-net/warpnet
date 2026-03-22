// Copyright (c) 2024 Mr. Gecko's Media (James Coleman). http://mrgeckosmedia.com/
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package selfupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	yaml "gopkg.in/yaml.v3"
)

type HttpManifest struct {
	LastReleaseID int64          `yaml:"last_release_id"`
	LastAssetID   int64          `yaml:"last_asset_id"`
	Releases      []*HttpRelease `yaml:"releases"`
}

// HttpConfig is an object to pass to NewHttpSource
type HttpConfig struct {
	// BaseURL is a base URL of your update server. This parameter has NO default value.
	BaseURL string
	// HTTP Transport Config
	Transport *http.Transport
	// Additional headers
	Headers http.Header
}

// HttpSource is used to load release information from an http repository
type HttpSource struct {
	baseURL   string
	transport *http.Transport
	headers   http.Header
}

// NewHttpSource creates a new HttpSource from a config object.
func NewHttpSource(config HttpConfig) (*HttpSource, error) {
	// Validate Base URL.
	if config.BaseURL == "" {
		return nil, fmt.Errorf("http base url must be set")
	}
	_, perr := url.ParseRequestURI(config.BaseURL)
	if perr != nil {
		return nil, perr
	}

	// Setup standard transport if not set.
	if config.Transport == nil {
		config.Transport = &http.Transport{}
	}

	// Return new source.
	return &HttpSource{
		baseURL:   config.BaseURL,
		transport: config.Transport,
		headers:   config.Headers,
	}, nil
}

// Returns a full URI for a relative path URI.
func (s *HttpSource) uriRelative(uri, owner, repo string) string {
	// If URI is blank, its blank.
	if uri != "" {
		// If we're able to parse the URI, a full URI is already defined.
		_, perr := url.ParseRequestURI(uri)
		if perr != nil {
			// Join the paths if possible to make a full URI.
			newURL, jerr := url.JoinPath(s.baseURL, owner, repo, uri)
			if jerr == nil {
				uri = newURL
			}
		}
	}
	return uri
}

// ListReleases returns all available releases
func (s *HttpSource) ListReleases(ctx context.Context, repository Repository) ([]SourceRelease, error) {
	owner, repo, err := repository.GetSlug()
	if err != nil {
		return nil, err
	}

	// Make repository URI.
	uri, err := url.JoinPath(s.baseURL, owner, repo, "manifest.yaml")
	if err != nil {
		return nil, err
	}

	// Setup HTTP client.
	client := &http.Client{Transport: s.transport}

	// Make repository request.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, err
	}

	// Add headers to request.
	req.Header = s.headers

	// Perform the request.
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("HTTP request failed with status code %d", res.StatusCode)
	}

	// Decode the response.
	manifest := new(HttpManifest)
	defer res.Body.Close()
	decoder := yaml.NewDecoder(res.Body)
	err = decoder.Decode(manifest)
	if err != nil {
		return nil, err
	}

	// Make a release array.
	releases := make([]SourceRelease, len(manifest.Releases))
	for i, release := range manifest.Releases {
		// Update URLs to relative path with repository.
		release.URL = s.uriRelative(release.URL, owner, repo)
		for b, asset := range release.Assets {
			release.Assets[b].URL = s.uriRelative(asset.URL, owner, repo)
		}

		// Set the release.
		releases[i] = release
	}

	return releases, nil
}

// DownloadReleaseAsset downloads an asset from a release.
// It returns an io.ReadCloser: it is your responsibility to Close it.
func (s *HttpSource) DownloadReleaseAsset(ctx context.Context, rel *Release, assetID int64) (io.ReadCloser, error) {
	if rel == nil {
		return nil, ErrInvalidRelease
	}

	// Determine download url based on asset id.
	var downloadUrl string
	if rel.AssetID == assetID {
		downloadUrl = rel.AssetURL
	} else if rel.ValidationAssetID == assetID {
		downloadUrl = rel.ValidationAssetURL
	}
	if downloadUrl == "" {
		return nil, fmt.Errorf("asset ID %d: %w", assetID, ErrAssetNotFound)
	}

	// Setup HTTP client.
	client := &http.Client{Transport: s.transport}

	// Make request.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadUrl, http.NoBody)
	if err != nil {
		return nil, err
	}

	// Add headers to request.
	req.Header = s.headers

	// Perform the request.
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("HTTP request failed with status code %d", response.StatusCode)
	}

	return response.Body, nil
}

// Verify interface
var _ Source = &HttpSource{}
