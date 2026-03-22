package selfupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// GitLabConfig is an object to pass to NewGitLabSource
type GitLabConfig struct {
	// APIToken represents GitLab API token. If it's not empty, it will be used for authentication for the API
	APIToken string
	// BaseURL is a base URL of your private GitLab instance
	BaseURL string
}

// GitLabSource is used to load release information from GitLab
type GitLabSource struct {
	api     *gitlab.Client
	token   string
	baseURL string
}

// NewGitLabSource creates a new GitLabSource from a config object.
// It initializes a GitLab API client.
// If you set your API token to the $GITLAB_TOKEN environment variable, the client will use it.
// You can pass an empty GitLabSource{} to use the default configuration
// The function will return an error if the GitLab Enterprise URLs in the config object cannot be parsed
func NewGitLabSource(config GitLabConfig) (*GitLabSource, error) {
	token := config.APIToken
	if token == "" {
		// try the environment variable
		token = os.Getenv("GITLAB_TOKEN")
	}
	option := make([]gitlab.ClientOptionFunc, 0, 1)
	if config.BaseURL != "" {
		option = append(option, gitlab.WithBaseURL(config.BaseURL))
	}
	client, err := gitlab.NewClient(token, option...)
	if err != nil {
		return nil, fmt.Errorf("cannot create GitLab client: %w", err)
	}
	return &GitLabSource{
		api:     client,
		token:   token,
		baseURL: config.BaseURL,
	}, nil
}

// ListReleases returns all available releases
func (s *GitLabSource) ListReleases(ctx context.Context, repository Repository) ([]SourceRelease, error) {
	pid, err := repository.Get()
	if err != nil {
		return nil, err
	}

	rels, _, err := s.api.Releases.ListReleases(pid, nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}
	releases := make([]SourceRelease, len(rels))
	for i, rel := range rels {
		releases[i] = NewGitLabRelease(rel)
	}
	return releases, nil
}

// DownloadReleaseAsset downloads an asset from a release.
// It returns an io.ReadCloser: it is your responsibility to Close it.
func (s *GitLabSource) DownloadReleaseAsset(ctx context.Context, rel *Release, assetID int64) (io.ReadCloser, error) {
	if rel == nil {
		return nil, ErrInvalidRelease
	}
	var downloadUrl string
	if rel.AssetID == assetID {
		downloadUrl = rel.AssetURL
	} else if rel.ValidationAssetID == assetID {
		downloadUrl = rel.ValidationAssetURL
	}
	if downloadUrl == "" {
		return nil, fmt.Errorf("asset ID %d: %w", assetID, ErrAssetNotFound)
	}

	log.Printf("downloading %q", downloadUrl)
	client := http.DefaultClient
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadUrl, http.NoBody)
	if err != nil {
		log.Print(err)
		return nil, err
	}

	if s.token != "" {
		// verify request is from same domain not to leak token
		ok, err := canUseTokenForDomain(s.baseURL, downloadUrl)
		if err != nil {
			return nil, err
		}
		if ok {
			req.Header.Set("PRIVATE-TOKEN", s.token)
		}
	}
	response, err := client.Do(req)

	if err != nil {
		log.Print(err)
		return nil, err
	}

	return response.Body, nil
}

// Verify interface
var _ Source = &GitLabSource{}
