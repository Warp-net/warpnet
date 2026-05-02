/*

 Warpnet - Decentralized Social Network
 Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
 <github.com.mecdy@passmail.net>

 This program is free software: you can redistribute it and/or modify
 it under the terms of the GNU Affero General Public License as published by
 the Free Software Foundation, either version 3 of the License, or
 (at your option) any later version.

 This program is distributed in the hope that it will be useful,
 but WITHOUT ANY WARRANTY; without even the implied warranty of
 MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 GNU Affero General Public License for more details.

 You should have received a copy of the GNU Affero General Public License
 along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

package warpnet

import (
	"embed"
	"io/fs"
)

// codeBaseSingleton embeds the project's Go source so the challenge
// handler can verify it. We list each top-level Go directory
// explicitly because `warpdroid/` is a separate Go module — a
// catch-all glob like `*/*/*.go` would match `warpdroid/node/*.go`
// and Go embed refuses to cross module boundaries.
//
//go:embed *.go
//go:embed cmd/*/*/*.go cmd/*/*/*/*.go
//go:embed config/*.go
//go:embed core/*/*.go
//go:embed database/*.go database/*/*.go
//go:embed domain/*.go
//go:embed event/*.go
//go:embed json/*.go
//go:embed metrics/*.go
//go:embed retrier/*.go
//go:embed security/*.go
var codeBaseSingleton embed.FS

func GetCodeBase() embed.FS {
	return codeBaseSingleton
}

//go:embed version
var versionSingleton []byte

func GetVersion() []byte {
	return versionSingleton
}

//go:embed cmd/node/member/icon.png
var logo []byte

func GetLogo() []byte {
	return logo
}

//go:embed frontend/dist
var static embed.FS

func GetStaticEmbedded() fs.FS {
	return static
}
