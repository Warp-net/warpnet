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

package security

import (
	"testing"

	root "github.com/Warp-net/warpnet"
)

func TestChallengeResolve_Success(t *testing.T) {
	codebase := root.GetCodeBase()
	nonce := int64(1)

	for i := 0; i < 10; i++ {
		sampleOrigin, location, err := GenerateChallenge(codebase, nonce)
		if err != nil {
			t.Fatal(err)
		}
		sampleFound, err := ResolveChallenge(codebase, location, nonce)
		if err != nil {
			t.Fatal(err)
		}

		if string(sampleFound) != string(sampleOrigin) {
			t.Fatalf("samples are not equal: \n %s %s", sampleOrigin, sampleFound)
		}
	}
}
