package selfupdate

import (
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GitLabRelease struct {
	releaseID   int64
	name        string
	tagName     string
	url         string
	publishedAt time.Time
	description string
	assets      []SourceAsset
}

func NewGitLabRelease(from *gitlab.Release) *GitLabRelease {
	release := &GitLabRelease{
		releaseID:   0,
		name:        from.Name,
		tagName:     from.TagName,
		url:         from.Commit.WebURL,
		publishedAt: *from.ReleasedAt,
		description: from.Description,
		assets:      make([]SourceAsset, len(from.Assets.Links)),
	}
	for i, fromLink := range from.Assets.Links {
		release.assets[i] = NewGitLabAsset(fromLink)
	}
	return release
}

func (r *GitLabRelease) GetID() int64 {
	return 0
}

func (r *GitLabRelease) GetTagName() string {
	return r.tagName
}

func (r *GitLabRelease) GetDraft() bool {
	return false
}

func (r *GitLabRelease) GetPrerelease() bool {
	return false
}

func (r *GitLabRelease) GetPublishedAt() time.Time {
	return r.publishedAt
}

func (r *GitLabRelease) GetReleaseNotes() string {
	return r.description
}

func (r *GitLabRelease) GetName() string {
	return r.name
}

func (r *GitLabRelease) GetURL() string {
	return r.url
}

func (r *GitLabRelease) GetAssets() []SourceAsset {
	return r.assets
}

type GitLabAsset struct {
	id   int64
	name string
	url  string
}

func NewGitLabAsset(from *gitlab.ReleaseLink) *GitLabAsset {
	return &GitLabAsset{
		id:   int64(from.ID),
		name: from.Name,
		url:  from.URL,
	}
}

func (a *GitLabAsset) GetID() int64 {
	return a.id
}

func (a *GitLabAsset) GetName() string {
	return a.name
}

func (a *GitLabAsset) GetSize() int {
	return 0
}

func (a *GitLabAsset) GetBrowserDownloadURL() string {
	return a.url
}

// Verify interface
var (
	_ SourceRelease = &GitLabRelease{}
	_ SourceAsset   = &GitLabAsset{}
)
