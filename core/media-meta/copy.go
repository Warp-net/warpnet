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

package media_meta

import (
	"encoding/base64"

	"github.com/Warp-net/warpnet/core/video"
	"github.com/Warp-net/warpnet/domain"
)

const videoFramesCount = 5

type VideoFrameEncoder interface {
	EncodeFrames(b []byte, samples []int, watermark []byte) ([]domain.VideoFrame, error)
}

func BuildImageCopy(original string, c domain.MediaCopy, signer Metadata) (string, error) {
	header, raw, err := SplitDataURI(original)
	if err != nil {
		return "", err
	}
	if len(c.Watermark) != 0 {
		var pixelMarkSeed []byte
		if c.IsPixelMarked {
			pixelMarkSeed = BuildPixelMarkSeed(signer.PrivKey, c.OriginalKey, c.BuyerId)
		}
		drawn, err := DrawWatermark(raw, c.Watermark, pixelMarkSeed)
		if err != nil {
			return "", err
		}
		if raw, err = signer.SignChangedJPEG(raw, drawn); err != nil {
			return "", err
		}
	}
	marked, err := EmbedOrderInJPEG(raw, c.EncryptedOrder)
	if err != nil {
		return "", err
	}
	return header + "," + base64.StdEncoding.EncodeToString(marked), nil
}

func BuildVideoFrames(original string, c domain.MediaCopy, signer Metadata, encoder VideoFrameEncoder) ([]domain.VideoFrame, error) {
	_, b, err := SplitDataURI(original)
	if err != nil {
		return nil, err
	}
	raw, _, err := SplitVideo(b)
	if err != nil {
		return nil, err
	}
	size, err := video.DisplaySize(raw)
	if err != nil {
		return nil, err
	}
	watermark, err := DrawVideoWatermark(size, c.BuyerId)
	if err != nil {
		return nil, err
	}
	samples, err := video.PickFrames(raw, BuildPixelMarkSeed(signer.PrivKey, c.OriginalKey, c.BuyerId), videoFramesCount)
	if err != nil {
		return nil, err
	}
	return encoder.EncodeFrames(raw, samples, watermark)
}

func BuildVideoCopy(original string, c domain.MediaCopy, signer Metadata) (string, error) {
	header, b, err := SplitDataURI(original)
	if err != nil {
		return "", err
	}
	if len(c.Frames) != 0 {
		raw, _, err := SplitVideo(b)
		if err != nil {
			return "", err
		}
		spliced, err := video.InsertFrames(raw, c.Frames)
		if err != nil {
			return "", err
		}
		if b, err = signer.SignChangedVideo(b, spliced); err != nil {
			return "", err
		}
	}
	marked, err := EmbedOrderInVideo(b, c.EncryptedOrder)
	if err != nil {
		return "", err
	}
	return header + "," + base64.StdEncoding.EncodeToString(marked), nil
}
