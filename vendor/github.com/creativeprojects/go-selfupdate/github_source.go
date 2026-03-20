package selfupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/google/go-github/v74/github"
	"golang.org/x/oauth2"
)

// GitHubConfig is an object to pass to NewGitHubSource
type GitHubConfig struct {
	// APIToken represents GitHub API token. If it's not empty, it will be used for authentication of GitHub API
	APIToken string
	// EnterpriseBaseURL is a base URL of GitHub API. If you want to use this library with GitHub Enterprise,
	// please set "https://{your-organization-address}/api/v3/" to this field.
	EnterpriseBaseURL string
	// EnterpriseUploadURL is a URL to upload stuffs to GitHub Enterprise instance. This is often the same as an API base URL.
	// So if this field is not set and EnterpriseBaseURL is set, EnterpriseBaseURL is also set to this field.
	EnterpriseUploadURL string
	// Deprecated: Context option is no longer used
	Context context.Context
}

// GitHubSource is used to load release information from GitHub
type GitHubSource struct {
	api *github.Client
}

// NewGitHubSource creates a new GitHubSource from a config object.
// It initializes a GitHub API client.
// If you set your API token to the $GITHUB_TOKEN environment variable, the client will use it.
// You can pass an empty GitHubSource{} to use the default configuration
// The function will return an error if the GitHub Enterprise URLs in the config object cannot be parsed
func NewGitHubSource(config GitHubConfig) (*GitHubSource, error) {
	token := config.APIToken
	if token == "" {
		// try the environment variable
		token = os.Getenv("GITHUB_TOKEN")
	}
	hc := newHTTPClient(token)

	if config.EnterpriseBaseURL == "" {
		// public (or private) repository on standard GitHub offering
		client := github.NewClient(hc)
		return &GitHubSource{
			api: client,
		}, nil
	}

	u := config.EnterpriseUploadURL
	if u == "" {
		u = config.EnterpriseBaseURL
	}
	client, err := github.NewEnterpriseClient(config.EnterpriseBaseURL, u, hc)
	if err != nil {
		return nil, fmt.Errorf("cannot parse GitHub enterprise URL: %w", err)
	}
	return &GitHubSource{
		api: client,
	}, nil
}

// ListReleases returns all available releases
func (s *GitHubSource) ListReleases(ctx context.Context, repository Repository) ([]SourceRelease, error) {
	owner, repo, err := repository.GetSlug()
	if err != nil {
		return nil, err
	}
	rels, res, err := s.api.Repositories.ListReleases(ctx, owner, repo, nil)
	if err != nil {
		if res != nil && res.StatusCode == http.StatusNotFound {
			// repository not found or release not found. It's not an error here.
			log.Print("Repository or release not found")
			return nil, nil
		}
		log.Printf("API returned an error response: %s", err)
		return nil, err
	}
	releases := make([]SourceRelease, len(rels))
	for i, rel := range rels {
		releases[i] = NewGitHubRelease(rel)
	}
	return releases, nil
}

// DownloadReleaseAsset downloads an asset from a release.
// It returns an io.ReadCloser: it is your responsibility to Close it.
func (s *GitHubSource) DownloadReleaseAsset(ctx context.Context, rel *Release, assetID int64) (io.ReadCloser, error) {
	if rel == nil {
		return nil, ErrInvalidRelease
	}
	owner, repo, err := rel.repository.GetSlug()
	if err != nil {
		return nil, err
	}
	// create a new http client so the GitHub library can download the redirected file (if any)
	client := http.DefaultClient
	rc, _, err := s.api.Repositories.DownloadReleaseAsset(ctx, owner, repo, assetID, client)
	if err != nil {
		return nil, fmt.Errorf("failed to call GitHub Releases API for getting the asset ID %d on repository '%s/%s': %w", assetID, owner, repo, err)
	}
	return rc, nil
}

func newHTTPClient(token string) *http.Client {
	if token == "" {
		return http.DefaultClient
	}
	src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return oauth2.NewClient(context.Background(), src)
}

// Verify interface
var _ Source = &GitHubSource{}
