package selfupdate

import (
	"time"

	"github.com/google/go-github/v74/github"
)

type GitHubRelease struct {
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

func NewGitHubRelease(from *github.RepositoryRelease) *GitHubRelease {
	release := &GitHubRelease{
		releaseID:    from.GetID(),
		name:         from.GetName(),
		tagName:      from.GetTagName(),
		url:          from.GetHTMLURL(),
		publishedAt:  from.GetPublishedAt().Time,
		releaseNotes: from.GetBody(),
		draft:        from.GetDraft(),
		prerelease:   from.GetPrerelease(),
		assets:       make([]SourceAsset, len(from.Assets)),
	}
	for i, fromAsset := range from.Assets {
		release.assets[i] = NewGitHubAsset(fromAsset)
	}
	return release
}

func (a *GitHubRelease) GetID() int64 {
	return a.releaseID
}

func (r *GitHubRelease) GetTagName() string {
	return r.tagName
}

func (r *GitHubRelease) GetDraft() bool {
	return r.draft
}

func (r *GitHubRelease) GetPrerelease() bool {
	return r.prerelease
}

func (r *GitHubRelease) GetPublishedAt() time.Time {
	return r.publishedAt
}

func (r *GitHubRelease) GetReleaseNotes() string {
	return r.releaseNotes
}

func (r *GitHubRelease) GetName() string {
	return r.name
}

func (r *GitHubRelease) GetURL() string {
	return r.url
}

func (r *GitHubRelease) GetAssets() []SourceAsset {
	return r.assets
}

type GitHubAsset struct {
	id   int64
	name string
	size int
	url  string
}

func NewGitHubAsset(from *github.ReleaseAsset) *GitHubAsset {
	return &GitHubAsset{
		id:   from.GetID(),
		name: from.GetName(),
		size: from.GetSize(),
		url:  from.GetBrowserDownloadURL(),
	}
}

func (a *GitHubAsset) GetID() int64 {
	return a.id
}

func (a *GitHubAsset) GetName() string {
	return a.name
}

func (a *GitHubAsset) GetSize() int {
	return a.size
}

func (a *GitHubAsset) GetBrowserDownloadURL() string {
	return a.url
}

// Verify interface
var (
	_ SourceRelease = &GitHubRelease{}
	_ SourceAsset   = &GitHubAsset{}
)
