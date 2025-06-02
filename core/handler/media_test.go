/*

Warpnet - Decentralized Social Network
Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
<github.com.mecdy@passmail.net>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

WarpNet is provided “as is” without warranty of any kind, either expressed or implied.
Use at your own risk. The maintainers shall not be liable for any damages or data loss
resulting from the use or misuse of this software.
*/

// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: gpl

package handler

import (
	"bytes"
	"encoding/base64"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/database"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	jis "github.com/dsoprea/go-jpeg-image-structure/v2"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/assert"
	"image"
	"image/jpeg"
	"strings"
	"testing"
	"time"
)

const testImagePNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABgAAAAYCAYAAADgdz34AAAABHNCSVQICAgIfAhkiAAAAAlwSFlzAAAApgAAAKYB3X3/OAAAABl0RVh0U29mdHdhcmUAd3d3Lmlua3NjYXBlLm9yZ5vuPBoAAANCSURBVEiJtZZPbBtFFMZ/M7ubXdtdb1xSFyeilBapySVU8h8OoFaooFSqiihIVIpQBKci6KEg9Q6H9kovIHoCIVQJJCKE1ENFjnAgcaSGC6rEnxBwA04Tx43t2FnvDAfjkNibxgHxnWb2e/u992bee7tCa00YFsffekFY+nUzFtjW0LrvjRXrCDIAaPLlW0nHL0SsZtVoaF98mLrx3pdhOqLtYPHChahZcYYO7KvPFxvRl5XPp1sN3adWiD1ZAqD6XYK1b/dvE5IWryTt2udLFedwc1+9kLp+vbbpoDh+6TklxBeAi9TL0taeWpdmZzQDry0AcO+jQ12RyohqqoYoo8RDwJrU+qXkjWtfi8Xxt58BdQuwQs9qC/afLwCw8tnQbqYAPsgxE1S6F3EAIXux2oQFKm0ihMsOF71dHYx+f3NND68ghCu1YIoePPQN1pGRABkJ6Bus96CutRZMydTl+TvuiRW1m3n0eDl0vRPcEysqdXn+jsQPsrHMquGeXEaY4Yk4wxWcY5V/9scqOMOVUFthatyTy8QyqwZ+kDURKoMWxNKr2EeqVKcTNOajqKoBgOE28U4tdQl5p5bwCw7BWquaZSzAPlwjlithJtp3pTImSqQRrb2Z8PHGigD4RZuNX6JYj6wj7O4TFLbCO/Mn/m8R+h6rYSUb3ekokRY6f/YukArN979jcW+V/S8g0eT/N3VN3kTqWbQ428m9/8k0P/1aIhF36PccEl6EhOcAUCrXKZXXWS3XKd2vc/TRBG9O5ELC17MmWubD2nKhUKZa26Ba2+D3P+4/MNCFwg59oWVeYhkzgN/JDR8deKBoD7Y+ljEjGZ0sosXVTvbc6RHirr2reNy1OXd6pJsQ+gqjk8VWFYmHrwBzW/n+uMPFiRwHB2I7ih8ciHFxIkd/3Omk5tCDV1t+2nNu5sxxpDFNx+huNhVT3/zMDz8usXC3ddaHBj1GHj/As08fwTS7Kt1HBTmyN29vdwAw+/wbwLVOJ3uAD1wi/dUH7Qei66PfyuRj4Ik9is+hglfbkbfR3cnZm7chlUWLdwmprtCohX4HUtlOcQjLYCu+fzGJH2QRKvP3UNz8bWk1qMxjGTOMThZ3kvgLI5AzFfo379UAAAAASUVORK5CYII="

func TestUploadImage_Success(t *testing.T) {
	ev := event.UploadImageEvent{
		File: testImagePNG,
	}
	bt, err := json.JSON.Marshal(ev)
	assert.NoError(t, err)

	_, err = StreamUploadImageHandler(n{}, m{}, u{})(bt, s{})
	assert.NoError(t, err)
}

const (
	testMetaTag   = imageDescriptionTag
	testMetaValue = "test meta value"
)

func TestAmendExif_Success(t *testing.T) {
	parts := strings.SplitN(testImagePNG, ",", 2)

	imgBytes, err := base64.StdEncoding.DecodeString(parts[1])
	assert.NoError(t, err)

	img, _, err := image.Decode(bytes.NewReader(imgBytes))
	assert.NoError(t, err)

	var imageBuf bytes.Buffer
	err = jpeg.Encode(&imageBuf, img, &jpeg.Options{Quality: 100})
	assert.NoError(t, err)

	metaBytes := []byte(testMetaValue)

	result, err := amendExifMetadata(imageBuf.Bytes(), metaBytes)
	assert.NoError(t, err)

	validateExif(t, result)
	assert.NoError(t, err)
}

func validateExif(t *testing.T, data []byte) {
	parser := jis.NewJpegMediaParser()

	intfc, err := parser.ParseBytes(data)
	assert.NoError(t, err)

	sl, ok := intfc.(*jis.SegmentList)
	assert.True(t, ok, "validate: invalid exif type: not a segment list")

	_, _, exifTags, err := sl.DumpExif()
	assert.NoError(t, err)

	var isFound bool
	for _, et := range exifTags {
		decoded, err := base64.StdEncoding.DecodeString(et.FormattedFirst)
		assert.NoError(t, err)

		if et.TagName == testMetaTag {
			assert.Equal(t, testMetaValue, string(decoded))
			isFound = true
			break
		}
	}

	assert.True(t, isFound, "validate: meta data not found")
}

type (
	n struct{}
	m struct{}
	u struct{}
	s struct{}
)

func (u u) GetOtherNetworkUser(network, userId string) (user domain.User, err error) {
	return domain.User{}, err
}

func (m m) SetForeignImageWithTTL(userId, key string, img database.Base64Image) error {
	return nil
}

func (m m) GetImage(userId, key string) (database.Base64Image, error) {
	return "", nil
}

func (m m) SetImage(userId string, img database.Base64Image) (key database.ImageKey, err error) {
	return "", nil
}

func (n n) NodeInfo() warpnet.NodeInfo {
	return warpnet.NodeInfo{}
}

func (s s) Read(p []byte) (n int, err error) {
	return 0, nil
}

func (s s) Write(p []byte) (n int, err error) {
	return 0, nil
}

func (s s) Close() error {
	return nil
}

func (s s) CloseWrite() error {
	return nil
}

func (s s) CloseRead() error {
	return nil
}

func (s s) Reset() error {
	return nil
}

func (s s) ResetWithError(errCode network.StreamErrorCode) error {
	return nil
}

func (s s) SetDeadline(time time.Time) error {
	return nil
}

func (s s) SetReadDeadline(time time.Time) error {
	return nil
}

func (s s) SetWriteDeadline(time time.Time) error {
	return nil
}

func (s s) ID() string {
	return ""
}

func (s s) Protocol() protocol.ID {
	return ""
}

func (s s) SetProtocol(id protocol.ID) error {
	return nil
}

func (s s) Stat() network.Stats {
	return network.Stats{}
}

func (s s) Conn() network.Conn {
	return nil
}

func (s s) Scope() network.StreamScope {
	return nil
}

func (u u) Get(userId string) (user domain.User, err error) {
	return domain.User{}, nil
}
