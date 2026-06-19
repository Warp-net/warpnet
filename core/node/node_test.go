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

package node

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
)

func TestIsBenignStreamCloseErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil means fully read", err: nil, want: true},
		{name: "io.EOF is benign", err: io.EOF, want: true},
		{name: "io.ErrClosedPipe is benign", err: io.ErrClosedPipe, want: true},
		{name: "wrapped io.EOF is benign", err: fmt.Errorf("read: %w", io.EOF), want: true},
		{name: "wrapped io.ErrClosedPipe is benign", err: fmt.Errorf("read: %w", io.ErrClosedPipe), want: true},
		{name: "deadline exceeded is a real failure", err: context.DeadlineExceeded, want: false},
		{name: "reset is a real failure", err: warpnet.WarpError("stream reset"), want: false},
		{name: "unexpected EOF is a real failure", err: io.ErrUnexpectedEOF, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBenignStreamCloseErr(tt.err); got != tt.want {
				t.Fatalf("isBenignStreamCloseErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
