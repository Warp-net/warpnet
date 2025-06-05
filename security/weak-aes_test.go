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

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

package security

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAESEncryptDecrypt_Success(t *testing.T) {
	password := []byte("SuperSecretPassword123!")
	in := []byte("Hello, this is a secret message.")

	cipherData, err := EncryptAES(in, password)
	assert.NoError(t, err)

	out, err := decryptAES(cipherData, password)
	assert.NoError(t, err)

	assert.Equal(t, in, out)
}
