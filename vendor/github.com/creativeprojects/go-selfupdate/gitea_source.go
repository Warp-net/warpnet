package selfupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"code.gitea.io/sdk/gitea"
)

// GiteaConfig is an object to pass to NewGiteaSource
type GiteaConfig struct {
	// APIToken represents Gitea API token. If it's not empty, it will be used for authentication for the API
	APIToken string
	// BaseURL is a base URL of your gitea instance. This parameter has NO default value.
	BaseURL string
	// Deprecated: Context option is no longer used
	Context context.Context
}

// GiteaSource is used to load release information from Gitea
type GiteaSource struct {
	api     *gitea.Client
	token   string
	baseURL string
}

// NewGiteaSource creates a new NewGiteaSource from a config object.
// It initializes a Gitea API Client.
// If you set your API token to the $GITEA_TOKEN environment variable, the client will use it.
// You can pass an empty GiteaSource{} to use the default configuration
func NewGiteaSource(config GiteaConfig) (*GiteaSource, error) {
	token := config.APIToken
	if token == "" {
		// try the environment variable
		token = os.Getenv("GITEA_TOKEN")
	}
	if config.BaseURL == "" {
		return nil, fmt.Errorf("gitea base url must be set")
	}

	ctx := config.Context
	if ctx == nil {
		ctx = context.Background()
	}

	client, err := gitea.NewClient(config.BaseURL, gitea.SetContext(ctx), gitea.SetToken(token))
	if err != nil {
		return nil, fmt.Errorf("error connecting to gitea: %w", err)
	}

	return &GiteaSource{
		api:     client,
		token:   token,
		baseURL: config.BaseURL,
	}, nil
}

// ListReleases returns all available releases
func (s *GiteaSource) ListReleases(ctx context.Context, repository Repository) ([]SourceRelease, error) {
	owner, repo, err := repository.GetSlug()
	if err != nil {
		return nil, err
	}

	s.api.SetContext(ctx)
	rels, res, err := s.api.ListReleases(owner, repo, gitea.ListReleasesOptions{})
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
		releases[i] = NewGiteaRelease(rel)
	}
	return releases, nil
}

// DownloadReleaseAsset downloads an asset from a release.
// It returns an io.ReadCloser: it is your responsibility to Close it.
func (s *GiteaSource) DownloadReleaseAsset(ctx context.Context, rel *Release, assetID int64) (io.ReadCloser, error) {
	if rel == nil {
		return nil, ErrInvalidRelease
	}
	owner, repo, err := rel.repository.GetSlug()
	if err != nil {
		return nil, err
	}
	s.api.SetContext(ctx)
	attachment, _, err := s.api.GetReleaseAttachment(owner, repo, rel.ReleaseID, assetID)
	if err != nil {
		return nil, fmt.Errorf("failed to call Gitea Releases API for getting the asset ID %d on repository '%s/%s': %w", assetID, owner, repo, err)
	}

	client := http.DefaultClient
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, attachment.DownloadURL, http.NoBody)
	if err != nil {
		log.Print(err)
		return nil, err
	}

	if s.token != "" {
		// verify request is from same domain not to leak token
		ok, err := canUseTokenForDomain(s.baseURL, attachment.DownloadURL)
		if err != nil {
			return nil, err
		}
		if ok {
			req.Header.Set("Authorization", "token "+s.token)
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
var _ Source = &GiteaSource{}
