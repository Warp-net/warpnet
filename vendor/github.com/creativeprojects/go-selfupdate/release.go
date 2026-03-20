package selfupdate

import (
	"time"

	"github.com/Masterminds/semver/v3"
)

// Release represents a release asset for current OS and arch.
type Release struct {
	// AssetURL is a URL to the uploaded file for the release
	AssetURL string
	// AssetSize represents the size of asset in bytes
	AssetByteSize int
	// AssetID is the ID of the asset on the source platform
	AssetID int64
	// ReleaseID is the ID of the release on the source platform
	ReleaseID int64
	// AssetName is the filename of the asset
	AssetName string
	// ValidationAssetID is the ID of additional validation asset on the source platform
	ValidationAssetID int64
	// ValidationAssetURL is the URL of additional validation asset on the source platform
	ValidationAssetURL string
	// ValidationChain is the list of validation assets being used (first record is ValidationAssetID).
	ValidationChain []struct {
		// ValidationAssetID is the ID of additional validation asset on the source platform
		ValidationAssetID int64
		// ValidationAssetURL is the filename of additional validation asset on the source platform
		ValidationAssetName string
		// ValidationAssetURL is the URL of additional validation asset on the source platform
		ValidationAssetURL string
	}
	// URL is a URL to release page for browsing
	URL string
	// ReleaseNotes is a release notes of the release
	ReleaseNotes string
	// Name represents a name of the release
	Name string
	// PublishedAt is the time when the release was published
	PublishedAt time.Time
	// OS this release is for
	OS string
	// Arch this release is for
	Arch string
	// Arm 32bits version (if any). Valid values are 0 (unknown), 5, 6 or 7
	Arm uint8
	// Prerelease is set to true for alpha, beta or release candidates
	Prerelease bool
	// version is the parsed *semver.Version
	version    *semver.Version
	repository Repository
}

// Version is the version string of the release
func (r Release) Version() string {
	return r.version.String()
}

// Give access to some of the method of the internal semver
// so we can change the version without breaking compatibility

// Equal tests if two versions are equal to each other.
func (r Release) Equal(other string) bool {
	return r.version.Equal(semver.MustParse(other))
}

// LessThan tests if one version is less than another one.
func (r Release) LessThan(other string) bool {
	return r.version.LessThan(semver.MustParse(other))
}

// GreaterThan tests if one version is greater than another one.
func (r Release) GreaterThan(other string) bool {
	return r.version.GreaterThan(semver.MustParse(other))
}

// LessOrEqual tests if one version is less than or equal to another one.
func (r Release) LessOrEqual(other string) bool {
	return r.version.Compare(semver.MustParse(other)) <= 0
}

// GreaterOrEqual tests if one version is greater than or equal to another one.
func (r Release) GreaterOrEqual(other string) bool {
	return r.version.Compare(semver.MustParse(other)) >= 0
}
