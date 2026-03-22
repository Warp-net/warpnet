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
	"time"
)

type HttpAsset struct {
	ID   int64  `yaml:"id"`
	Name string `yaml:"name"`
	Size int    `yaml:"size"`
	URL  string `yaml:"url"`
}

func (a *HttpAsset) GetID() int64 {
	return a.ID
}

func (a *HttpAsset) GetName() string {
	return a.Name
}

func (a *HttpAsset) GetSize() int {
	return a.Size
}

func (a *HttpAsset) GetBrowserDownloadURL() string {
	return a.URL
}

var _ SourceAsset = &HttpAsset{}

type HttpRelease struct {
	ID           int64        `yaml:"id"`
	Name         string       `yaml:"name"`
	TagName      string       `yaml:"tag_name"`
	URL          string       `yaml:"url"`
	Draft        bool         `yaml:"draft"`
	Prerelease   bool         `yaml:"prerelease"`
	PublishedAt  time.Time    `yaml:"published_at"`
	ReleaseNotes string       `yaml:"release_notes"`
	Assets       []*HttpAsset `yaml:"assets"`
}

func (r *HttpRelease) GetID() int64 {
	return r.ID
}

func (r *HttpRelease) GetTagName() string {
	return r.TagName
}

func (r *HttpRelease) GetDraft() bool {
	return r.Draft
}

func (r *HttpRelease) GetPrerelease() bool {
	return r.Prerelease
}

func (r *HttpRelease) GetPublishedAt() time.Time {
	return r.PublishedAt
}

func (r *HttpRelease) GetReleaseNotes() string {
	return r.ReleaseNotes
}

func (r *HttpRelease) GetName() string {
	return r.Name
}

func (r *HttpRelease) GetURL() string {
	return r.URL
}

func (r *HttpRelease) GetAssets() []SourceAsset {
	assets := make([]SourceAsset, len(r.Assets))
	for i, asset := range r.Assets {
		assets[i] = asset
	}
	return assets
}

var _ SourceRelease = &HttpRelease{}
