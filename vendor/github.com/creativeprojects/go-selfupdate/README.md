Self-Update library for Github, Gitea and Gitlab hosted applications in Go
==============================================================

[![Godoc reference](https://godoc.org/github.com/creativeprojects/go-selfupdate?status.svg)](http://godoc.org/github.com/creativeprojects/go-selfupdate)
[![Build](https://github.com/creativeprojects/go-selfupdate/workflows/Build/badge.svg)](https://github.com/creativeprojects/go-selfupdate/actions)
[![codecov](https://codecov.io/gh/creativeprojects/go-selfupdate/branch/main/graph/badge.svg?token=3FejM0fkw2)](https://codecov.io/gh/creativeprojects/go-selfupdate)
[![Bugs](https://sonarcloud.io/api/project_badges/measure?project=creativeprojects_go-selfupdate&metric=bugs)](https://sonarcloud.io/summary/new_code?id=creativeprojects_go-selfupdate)
[![Reliability Rating](https://sonarcloud.io/api/project_badges/measure?project=creativeprojects_go-selfupdate&metric=reliability_rating)](https://sonarcloud.io/summary/new_code?id=creativeprojects_go-selfupdate)
[![Maintainability Rating](https://sonarcloud.io/api/project_badges/measure?project=creativeprojects_go-selfupdate&metric=sqale_rating)](https://sonarcloud.io/summary/new_code?id=creativeprojects_go-selfupdate)
[![Security Rating](https://sonarcloud.io/api/project_badges/measure?project=creativeprojects_go-selfupdate&metric=security_rating)](https://sonarcloud.io/summary/new_code?id=creativeprojects_go-selfupdate)
[![Vulnerabilities](https://sonarcloud.io/api/project_badges/measure?project=creativeprojects_go-selfupdate&metric=vulnerabilities)](https://sonarcloud.io/summary/new_code?id=creativeprojects_go-selfupdate)

<!--ts-->
* [Self\-Update library for Github, Gitea and Gitlab hosted applications in Go](#self-update-library-for-github-gitea-and-gitlab-hosted-applications-in-go)
* [Introduction](#introduction)
* [Example](#example)
* [Upgrade from v0\+ to v1](#upgrade-from-v0-to-v1)
  * [Repository](#repository)
    * [ParseSlug](#parseslug)
    * [NewRepositorySlug](#newrepositoryslug)
    * [NewRepositoryID (GitLab only)](#newrepositoryid-gitlab-only)
    * [Context](#context)
  * [Package functions](#package-functions)
  * [Methods on Source interface](#methods-on-source-interface)
  * [Methods on Updater struct](#methods-on-updater-struct)
* [Naming Rules of Released Binaries](#naming-rules-of-released-binaries)
* [Naming Rules of Versions (=Git Tags)](#naming-rules-of-versions-git-tags)
* [Structure of Releases](#structure-of-releases)
* [Special case for ARM architecture](#special-case-for-arm-architecture)
* [Hash or Signature Validation](#hash-or-signature-validation)
  * [SHA256](#sha256)
  * [ECDSA](#ecdsa)
  * [Using a single checksum file for all your assets](#using-a-single-checksum-file-for-all-your-assets)
* [macOS universal binaries](#macos-universal-binaries)
* [Other providers than Github](#other-providers-than-github)
* [GitLab](#gitlab)
  * [Example:](#example-1)
* [Http Based Repository](#http-based-repository)
  * [Example:](#example-2)
* [Copyright](#copyright)

<!--te-->

# Introduction

`go-selfupdate` detects the information of the latest release via a source provider and
checks the current version. If a newer version than itself is detected, it downloads the released binary from
the source provider and replaces itself.

- Automatically detect the latest version of released binary on the source provider
- Retrieve the proper binary for the OS and arch where the binary is running
- Update the binary with rollback support on failure
- Tested on Linux, macOS and Windows
- Support for different versions of ARM architecture
- Support macOS universal binaries
- Many archive and compression formats are supported (zip, tar, gzip, xz, bzip2)
- Support private repositories
- Support hash, signature validation

Three source providers are available:
- GitHub
- Gitea
- Gitlab

This library started as a fork of https://github.com/rhysd/go-github-selfupdate. A few things have changed from the original implementation:
- don't expose an external `semver.Version` type, but provide the same functionality through the API: `LessThan`, `Equal` and `GreaterThan`
- use an interface to send logs (compatible with standard log.Logger)
- able to detect different ARM CPU architectures (the original library wasn't working on my different versions of raspberry pi)
- support for assets compressed with bzip2 (.bz2)
- can use a single file containing the sha256 checksums for all the files (one per line)
- separate the provider and the updater, so we can add more providers (Github, Gitea, Gitlab, etc.)
- return well defined wrapped errors that can be checked with `errors.Is(err error, target error)`

# Example

Here's an example how to use the library for an application to update itself

```go
func update(version string) error {
	latest, found, err := selfupdate.DetectLatest(context.Background(), selfupdate.ParseSlug("creativeprojects/resticprofile"))
	if err != nil {
		return fmt.Errorf("error occurred while detecting version: %w", err)
	}
	if !found {
		return fmt.Errorf("latest version for %s/%s could not be found from github repository", runtime.GOOS, runtime.GOARCH)
	}

	if latest.LessOrEqual(version) {
		log.Printf("Current version (%s) is the latest", version)
		return nil
	}

	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		return errors.New("could not locate executable path")
	}
	if err := selfupdate.UpdateTo(context.Background(), latest.AssetURL, latest.AssetName, exe); err != nil {
		return fmt.Errorf("error occurred while updating binary: %w", err)
	}
	log.Printf("Successfully updated to version %s", latest.Version())
	return nil
}
```

# Upgrade from v0+ to v1

Version v1+ has a **stable** API. It is slightly different from the API of versions 0+.

## Repository

Some functions needed a couple `owner`/`repo` and some other a single string called `slug`. These have been replaced by a `Repository`.

Two constructors are available:

### ParseSlug

Parses a *slug* string like `owner/repository_name`

```go
func ParseSlug(slug string) RepositorySlug
```

### NewRepositorySlug

Creates a repository from both owner and repo strings

```go
func NewRepositorySlug(owner, repo string) RepositorySlug
```

### NewRepositoryID (GitLab only)

GitLab can also refer to a repository via its internal ID. This constructor can be used with a numeric repository ID.

```go
func NewRepositoryID(id int) RepositoryID
```

### Context

All methods are now accepting a `context` as their first parameter. You can use it to cancel a long running operation.

## Package functions

| v0 | v1 |
|----|----|
| UpdateTo(assetURL, assetFileName, cmdPath string) error | UpdateTo(ctx context.Context, assetURL, assetFileName, cmdPath string) error |
| DetectLatest(slug string) (*Release, bool, error) | DetectLatest(ctx context.Context, repository Repository) (*Release, bool, error) |
| DetectVersion(slug string, version string) (*Release, bool, error) | DetectVersion(ctx context.Context, repository Repository, version string) (*Release, bool, error) |
| UpdateCommand(cmdPath string, current string, slug string) (*Release, error) | UpdateCommand(ctx context.Context, cmdPath string, current string, repository Repository) (*Release, error) |
| UpdateSelf(current string, slug string) (*Release, error) | UpdateSelf(ctx context.Context, current string, repository Repository) (*Release, error) |

## Methods on Source interface

| v0 | v1 |
|----|----|
| ListReleases(owner, repo string) ([]SourceRelease, error) | ListReleases(ctx context.Context, repository Repository) ([]SourceRelease, error) |
| DownloadReleaseAsset(owner, repo string, releaseID, id int64) (io.ReadCloser, error) | DownloadReleaseAsset(ctx context.Context, rel *Release, assetID int64) (io.ReadCloser, error) |

## Methods on Updater struct

| v0 | v1 |
|----|----|
| DetectLatest(slug string) (release *Release, found bool, err error) | DetectLatest(ctx context.Context, repository Repository) (release *Release, found bool, err error) |
| DetectVersion(slug string, version string) (release *Release, found bool, err error) | DetectVersion(ctx context.Context, repository Repository, version string) (release *Release, found bool, err error) |
| UpdateCommand(cmdPath string, current string, slug string) (*Release, error) | UpdateCommand(ctx context.Context, cmdPath string, current string, repository Repository) (*Release, error) |
| UpdateSelf(current string, slug string) (*Release, error) | UpdateSelf(ctx context.Context, current string, repository Repository) (*Release, error) |
| UpdateTo(rel *Release, cmdPath string) error | UpdateTo(ctx context.Context, rel *Release, cmdPath string) error |


# Naming Rules of Released Binaries

go-selfupdate assumes that released binaries are put for each combination of platforms and architectures.
Binaries for each platform can be easily built using tools like [goreleaser][]

You need to put the binaries with the following format.

```
{cmd}_{goos}_{goarch}{.ext}
```

`{cmd}` is a name of command.
`{goos}` and `{goarch}` are the platform and the arch type of the binary.
`{.ext}` is a file extension. go-selfupdate supports `.zip`, `.gzip`, `.bz2`, `.tar.gz` and `.tar.xz`.
You can also use blank and it means binary is not compressed.

If you compress binary, uncompressed directory or file must contain the executable named `{cmd}`.

And you can also use `-` for separator instead of `_` if you like.

For example, if your command name is `foo-bar`, one of followings is expected to be put in release
page on GitHub as binary for platform `linux` and arch `amd64`.

- `foo-bar_linux_amd64` (executable)
- `foo-bar_linux_amd64.zip` (zip file)
- `foo-bar_linux_amd64.tar.gz` (tar file)
- `foo-bar_linux_amd64.xz` (xzip file)
- `foo-bar-linux-amd64.tar.gz` (`-` is also ok for separator)

If you compress and/or archive your release asset, it must contain an executable named one of followings:

- `foo-bar` (only command name)
- `foo-bar_linux_amd64` (full name)
- `foo-bar-linux-amd64` (`-` is also ok for separator)

To archive the executable directly on Windows, `.exe` can be added before file extension like
`foo-bar_windows_amd64.exe.zip`.

[goreleaser]: https://github.com/goreleaser/goreleaser/


# Naming Rules of Versions (=Git Tags)

go-selfupdate searches binaries' versions via Git tag names (not a release title).
When your tool's version is `1.2.3`, you should use the version number for tag of the Git
repository (i.e. `1.2.3` or `v1.2.3`).

This library assumes you adopt [semantic versioning][]. It is necessary for comparing versions
systematically.

Prefix before version number `\d+\.\d+\.\d+` is automatically omitted. For example, `ver1.2.3` or
`release-1.2.3` are also ok.

Tags which don't contain a version number are ignored (i.e. `nightly`). And releases marked as `pre-release`
are also ignored.

[semantic versioning]: https://semver.org/


# Structure of Releases

In summary, structure of releases on GitHub looks like:

- `v1.2.0`
  - `foo-bar-linux-amd64.tar.gz`
  - `foo-bar-linux-386.tar.gz`
  - `foo-bar-darwin-amd64.tar.gz`
  - `foo-bar-windows-amd64.zip`
  - ... (Other binaries for v1.2.0)
- `v1.1.3`
  - `foo-bar-linux-amd64.tar.gz`
  - `foo-bar-linux-386.tar.gz`
  - `foo-bar-darwin-amd64.tar.gz`
  - `foo-bar-windows-amd64.zip`
  - ... (Other binaries for v1.1.3)
- ... (older versions)

# Special case for ARM architecture

If you're using [goreleaser](https://github.com/goreleaser/goreleaser/) targeting ARM CPUs, it will use the version of the ARM architecture as a name:
- `armv5`
- `armv6`
- `armv7`

go-selfupdate will check which architecture was used to build the current binary. Please note it's **not detecting the hardware**, but the binary target instead. If you run an `armv6` binary on an `armv7` CPU, it will keep `armv6` as a target.

As a rule, it will search for a binary with the same architecture first, then try the architectures below if available, and as a last resort will try a simple `arm` architecture tag.

So if you're running a `armv6` binary, it will try these targets in order:
- `armv6`
- `armv5`
- `arm`

More information on targeting ARM cpu can be found here: [GoArm](https://github.com/golang/go/wiki/GoArm)

# Hash or Signature Validation

go-selfupdate supports hash or signature validation of the downloaded files. It comes
with support for sha256 hashes or ECDSA signatures. If you need something different,
you can implement the `Validator` interface with your own validation mechanism:

```go
// Validator represents an interface which enables additional validation of releases.
type Validator interface {
	// Validate validates release bytes against an additional asset bytes.
	// See SHAValidator or ECDSAValidator for more information.
	Validate(filename string, release, asset []byte) error
	// GetValidationAssetName returns the additional asset name containing the validation checksum.
	// The asset containing the checksum can be based on the release asset name
	// Please note if the validation file cannot be found, the DetectLatest and DetectVersion methods
	// will fail with a wrapped ErrValidationAssetNotFound error
	GetValidationAssetName(releaseFilename string) string
}
```

## SHA256

To verify the integrity by SHA256, generate a hash sum and save it within a file which has the
same naming as original file with the suffix `.sha256`.
For e.g. use sha256sum, the file `selfupdate/testdata/foo.zip.sha256` is generated with:
```shell
sha256sum foo.zip > foo.zip.sha256
```

## ECDSA
To verify the signature by ECDSA generate a signature and save it within a file which has the
same naming as original file with the suffix `.sig`.
For e.g. use openssl, the file `selfupdate/testdata/foo.zip.sig` is generated with:
```shell
openssl dgst -sha256 -sign Test.pem -out foo.zip.sig foo.zip
```

go-selfupdate makes use of go internal crypto package. Therefore the private key
has to be compatible with FIPS 186-3.

## Using a single checksum file for all your assets

Tools like [goreleaser][] produce a single checksum file for all your assets. A Validator is provided out of the box for this case:

```go
updater, _ := NewUpdater(Config{Validator: &ChecksumValidator{UniqueFilename: "checksums.txt"}})
```

# macOS universal binaries

You can ask the updater to choose a macOS universal binary as a fallback if the native architecture wasn't found.

You need to provide the architecture name for the universal binary in the `Config` struct:

```go
updater, _ := NewUpdater(Config{UniversalArch: "all"})
```

Default is empty, which means no fallback.

# Other providers than Github

This library can be easily extended by providing a new source and release implementation for any git provider
Currently implemented are 
- Github (default)
- Gitea
- Gitlab

# GitLab

Support for GitLab landed in version 1.0.0.

To be able to download assets from a private instance of GitLab, you have to publish your files to the [Generic Package Registry](https://docs.gitlab.com/ee/user/packages/package_registry/index.html).

If you're using goreleaser, you just need to add this option:

```yaml
# .goreleaser.yml
gitlab_urls:
  use_package_registry: true

```

See [goreleaser documentation](https://goreleaser.com/scm/gitlab/#generic-package-registry) for more information.

## Example:

```go
func update() {
	source, err := selfupdate.NewGitLabSource(selfupdate.GitLabConfig{
		BaseURL: "https://private.instance.on.gitlab.com/",
	})
	if err != nil {
		log.Fatal(err)
	}
	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		Source:    source,
		Validator: &selfupdate.ChecksumValidator{UniqueFilename: "checksums.txt"}, // checksum from goreleaser
	})
	if err != nil {
		log.Fatal(err)
	}
	release, found, err := updater.DetectLatest(context.Background(), selfupdate.NewRepositorySlug("owner", "cli-tool"))
	if err != nil {
		log.Fatal(err)
	}
	if !found {
		log.Print("Release not found")
		return
	}
	fmt.Printf("found release %s\n", release.Version())

	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		return errors.New("could not locate executable path")
	}
	err = updater.UpdateTo(context.Background(), release, exe)
	if err != nil {
		log.Fatal(err)
	}
}
```

# Http Based Repository

Support for http based repositories landed in version 1.4.0.

The HttpSource is designed to work with repositories built using [goreleaser-http-repo-builder](https://github.com/GRMrGecko/goreleaser-http-repo-builder?tab=readme-ov-file). This provides a simple way to add self-update support to software that is not open source, allowing you to host your own updates. It requires that you still use the owner/project url style, and you can set custom headers to be used with requests to authenticate.

## Example:

If your repository is at example.com/repo/project, then you'd use the following example.

```go
func update() {
	source, err := selfupdate.NewHttpSource(selfupdate.HttpConfig{
		BaseURL: "https://example.com/",
	})
	if err != nil {
		log.Fatal(err)
	}
	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		Source:    source,
		Validator: &selfupdate.ChecksumValidator{UniqueFilename: "checksums.txt"}, // checksum from goreleaser
	})
	if err != nil {
		log.Fatal(err)
	}
	release, found, err := updater.DetectLatest(context.Background(), selfupdate.NewRepositorySlug("repo", "project"))
	if err != nil {
		log.Fatal(err)
	}
	if !found {
		log.Print("Release not found")
		return
	}
	fmt.Printf("found release %s\n", release.Version())

	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		return errors.New("could not locate executable path")
	}
	err = updater.UpdateTo(context.Background(), release, exe)
	if err != nil {
		log.Fatal(err)
	}
}
```

# Copyright

This work is heavily based on:


- [go-github-selfupdate](https://github.com/rhysd/go-github-selfupdate): [Copyright (c) 2017 rhysd](https://github.com/rhysd/go-github-selfupdate/blob/master/LICENSE)

- [go-update](https://github.com/inconshreveable/go-update): [Copyright 2015 Alan Shreve](https://github.com/inconshreveable/go-update/blob/master/LICENSE)
