package selfupdate

import (
	"time"

	"code.gitea.io/sdk/gitea"
)

type GiteaRelease struct {
	releaseID    int64
	name         string
	tagName      string
	url          string
	draft        bool
	prerelease   bool
	publishedAt  time.Time
	releaseNotes string
	assets       []SourceAsset
}

func NewGiteaRelease(from *gitea.Release) *GiteaRelease {
	release := &GiteaRelease{
		releaseID:    from.ID,
		name:         from.Title,
		tagName:      from.TagName,
		url:          "", //FIXME: we kind of have no url ?
		publishedAt:  from.PublishedAt,
		releaseNotes: from.Note,
		draft:        from.IsDraft,
		prerelease:   from.IsPrerelease,
		assets:       make([]SourceAsset, len(from.Attachments)),
	}

	for i, fromAsset := range from.Attachments {
		release.assets[i] = NewGiteaAsset(fromAsset)
	}

	return release
}

func (r *GiteaRelease) GetID() int64 {
	return r.releaseID
}

func (r *GiteaRelease) GetTagName() string {
	return r.tagName
}

func (r *GiteaRelease) GetDraft() bool {
	return r.draft
}

func (r *GiteaRelease) GetPrerelease() bool {
	return r.prerelease
}

func (r *GiteaRelease) GetPublishedAt() time.Time {
	return r.publishedAt
}

func (r *GiteaRelease) GetReleaseNotes() string {
	return r.releaseNotes
}

func (r *GiteaRelease) GetName() string {
	return r.name
}

func (r *GiteaRelease) GetURL() string {
	return r.url
}

func (r *GiteaRelease) GetAssets() []SourceAsset {
	return r.assets
}

type GiteaAsset struct {
	id   int64
	name string
	size int
	url  string
}

func NewGiteaAsset(from *gitea.Attachment) *GiteaAsset {
	return &GiteaAsset{
		id:   from.ID,
		name: from.Name,
		size: int(from.Size),
		url:  from.DownloadURL,
	}
}

func (a *GiteaAsset) GetID() int64 {
	return a.id
}

func (a *GiteaAsset) GetName() string {
	return a.name
}

func (a *GiteaAsset) GetSize() int {
	return a.size
}

func (a *GiteaAsset) GetBrowserDownloadURL() string {
	return a.url
}

// Verify interface
var (
	_ SourceRelease = &GiteaRelease{}
	_ SourceAsset   = &GiteaAsset{}
)
