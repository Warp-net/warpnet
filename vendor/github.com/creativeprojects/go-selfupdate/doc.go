/*
go-selfupdate detects the information of the latest release via GitHub Releases API and checks the current version.
If newer version than itself is detected, it downloads released binary from GitHub and replaces itself.

- Automatically detects the latest version of released binary on GitHub

- Retrieve the proper binary for the OS and arch where the binary is running

- Update the binary with rollback support on failure

- Tested on Linux, macOS and Windows

- Many archive and compression formats are supported (zip, gzip, xzip, bzip2, tar)

There are some naming rules. Please read following links.

Naming Rules of Released Binaries:
  https://github.com/creativeprojects/go-selfupdate#naming-rules-of-released-binaries

Naming Rules of Git Tags:
  https://github.com/creativeprojects/go-selfupdate#naming-rules-of-versions-git-tags

This package is hosted on GitHub:
  https://github.com/creativeprojects/go-selfupdate

Small CLI tools as wrapper of this library are available also:
  https://github.com/creativeprojects/go-selfupdate/cmd/detect-latest-release
  https://github.com/creativeprojects/go-selfupdate/cmd/go-get-release
*/
package selfupdate
