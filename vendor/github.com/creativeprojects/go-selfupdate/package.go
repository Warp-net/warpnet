package selfupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// DetectLatest detects the latest release from the repository.
// This function is a shortcut version of updater.DetectLatest with the DefaultUpdater.
func DetectLatest(ctx context.Context, repository Repository) (*Release, bool, error) {
	//nolint:contextcheck
	return DefaultUpdater().DetectLatest(ctx, repository)
}

// DetectVersion detects the given release from the repository.
func DetectVersion(ctx context.Context, repository Repository, version string) (*Release, bool, error) {
	//nolint:contextcheck
	return DefaultUpdater().DetectVersion(ctx, repository, version)
}

// UpdateTo downloads an executable from assetURL and replaces the current binary with the downloaded one.
// This function is low-level API to update the binary. Because it does not use a source provider and downloads asset directly from the URL via HTTP,
// this function is not available to update a release for private repositories.
// cmdPath is a file path to command executable.
func UpdateTo(ctx context.Context, assetURL, assetFileName, cmdPath string) error {
	//nolint:contextcheck
	up := DefaultUpdater()
	src, err := downloadReleaseAssetFromURL(ctx, assetURL)
	if err != nil {
		return err
	}
	defer src.Close()
	return up.decompressAndUpdate(src, assetFileName, assetURL, cmdPath)
}

// UpdateCommand updates a given command binary to the latest version.
// This function is a shortcut version of updater.UpdateCommand using a DefaultUpdater()
func UpdateCommand(ctx context.Context, cmdPath string, current string, repository Repository) (*Release, error) {
	//nolint:contextcheck
	return DefaultUpdater().UpdateCommand(ctx, cmdPath, current, repository)
}

// UpdateSelf updates the running executable itself to the latest version.
// This function is a shortcut version of updater.UpdateSelf using a DefaultUpdater()
func UpdateSelf(ctx context.Context, current string, repository Repository) (*Release, error) {
	//nolint:contextcheck
	return DefaultUpdater().UpdateSelf(ctx, current, repository)
}

func downloadReleaseAssetFromURL(ctx context.Context, url string) (rc io.ReadCloser, err error) {
	client := http.DefaultClient
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	req.Header.Set("Accept", "*/*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download a release file from %s: %w", url, err)
	}
	if resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("failed to download a release file from %s: HTTP %d", url, resp.StatusCode)
	}
	return resp.Body, nil
}
