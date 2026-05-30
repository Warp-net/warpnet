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

package server

import (
	"github.com/Warp-net/warpnet/cmd/node/business/server/handlers"
	"github.com/Warp-net/warpnet/security"
)

// aesCodec is the dashboard channel's wire form: AES-256-GCM with the preshared
// key (sha256 of the launch password) when one is configured, plaintext
// otherwise. The is-first-run probe precedes the key and arrives in cleartext,
// so Decode reports per frame whether it was encrypted and Encode mirrors that
// on the reply.
type aesCodec struct{ key []byte }

var _ handlers.Codec = aesCodec{}

func (c aesCodec) Decode(frame []byte) (plain []byte, encrypted bool) {
	if len(c.key) == 0 {
		return frame, false
	}
	if p, err := security.AESGCMDecrypt(c.key, frame); err == nil {
		return p, true
	}
	return frame, false
}

func (c aesCodec) Encode(reply []byte, encrypted bool) ([]byte, error) {
	if !encrypted || len(c.key) == 0 {
		return reply, nil
	}
	return security.AESGCMEncrypt(c.key, reply)
}
