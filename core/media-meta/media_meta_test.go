// Copyright 2025 Vadim Filin
// SPDX-License-Identifier: AGPL-3.0-or-later

//nolint:all
package media_meta

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/json"
	"github.com/Warp-net/warpnet/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var signerKey, signerID = mustSigner("media-meta-test-seed")

func mustSigner(seed string) (ed25519.PrivateKey, warpnet.WarpPeerID) {
	priv, err := security.GenerateKeyFromSeed([]byte(seed))
	if err != nil {
		panic(err)
	}
	id, err := warpnet.IDFromPublicKey(priv.Public().(ed25519.PublicKey))
	if err != nil {
		panic(err)
	}
	return priv, id
}

func watermark(ownerId string) Watermark {
	return Watermark{
		PrivKey:       signerKey,
		NodeId:        signerID.String(),
		OwnerId:       ownerId,
		EncryptedMeta: []byte("encrypted forensic payload"),
	}
}

func testJPEG(t *testing.T, shade uint8) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, color.RGBA{R: shade, G: shade, B: shade, A: 255})
		}
	}

	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}))
	return buf.Bytes()
}

func minimalMP4() []byte {
	return []byte{
		0x00, 0x00, 0x00, 0x18, // box size = 24
		'f', 't', 'y', 'p',
		'i', 's', 'o', 'm', // major brand
		0x00, 0x00, 0x02, 0x00, // minor version
		'i', 's', 'o', 'm',
		'm', 'p', '4', '1',
	}
}

func watermarkedJPEG(t *testing.T, ownerId string, shade uint8) []byte {
	t.Helper()

	plain := testJPEG(t, shade)
	meta, err := watermark(ownerId).Sign(security.ConvertToSHA256(plain))
	require.NoError(t, err)

	out, err := EmbedInJPEG(plain, meta)
	require.NoError(t, err)
	return out
}

func watermarkedMP4(t *testing.T, ownerId string) []byte {
	t.Helper()

	meta, err := watermark(ownerId).Sign(security.ConvertToSHA256(minimalMP4()))
	require.NoError(t, err)

	out, err := EmbedInVideo(minimalMP4(), meta)
	require.NoError(t, err)
	return out
}

func TestWatermark_RefusesWithoutKeyOrIdentity(t *testing.T) {
	hash := security.ConvertToSHA256([]byte("bytes"))

	_, err := Watermark{NodeId: "node", OwnerId: "owner"}.Sign(hash)
	assert.ErrorIs(t, err, ErrNoSigningKey)

	_, err = Watermark{PrivKey: signerKey}.Sign(hash)
	assert.ErrorIs(t, err, ErrNoSigningIdentity)
}

func TestVerify_EncryptedMetaIsCoveredBySignature(t *testing.T) {
	plain := testJPEG(t, 0x40)
	hash := security.ConvertToSHA256(plain)

	watermarkBytes, err := watermark("alice").Sign(hash)
	require.NoError(t, err)
	require.NoError(t, verify(watermarkBytes, hash, signerID.String(), "alice"))

	var signed signedWatermark
	require.NoError(t, json.Unmarshal(watermarkBytes, &signed))
	signed.EncryptedMeta = []byte("somebody else's encrypted meta")
	swapped, err := json.Marshal(signed)
	require.NoError(t, err)

	assert.ErrorIs(t, verify(swapped, hash, signerID.String(), "alice"), ErrForgedMetadata)
}

func TestVerifyImage_HoldsOnlyForTheIdentityItWasWatermarkedFor(t *testing.T) {
	watermarked := watermarkedJPEG(t, "alice", 0x40)

	assert.NoError(t, VerifyImage(watermarked, signerID.String(), "alice"))

	assert.ErrorIs(t, VerifyImage(watermarked, signerID.String(), "mallory"), ErrForgedMetadata)

	_, otherNode := mustSigner("another-node")
	assert.ErrorIs(t, VerifyImage(watermarked, otherNode.String(), "alice"), ErrForgedMetadata)
}

func TestVerifyImage_UnstampedFileIsRefused(t *testing.T) {
	assert.ErrorIs(t, VerifyImage(testJPEG(t, 0x40), signerID.String(), "alice"), ErrNoMetadata)
}

func TestVerifyImage_LegacyEncryptedMetaIsRefused(t *testing.T) {
	password, err := security.NewWeakPassword()
	require.NoError(t, err)

	encryptedMeta, err := security.EncryptAES([]byte(`{"node":"whoever"}`), password)
	require.NoError(t, err)

	legacy, err := EmbedInJPEG(testJPEG(t, 0x40), encryptedMeta)
	require.NoError(t, err)

	assert.ErrorIs(t, VerifyImage(legacy, signerID.String(), "alice"), ErrNoMetadata)
}

func TestVerifyImage_BlobForgedByAnotherNodeIsRefused(t *testing.T) {
	attackerKey, attackerID := mustSigner("attacker")

	claim := Watermark{
		PrivKey:       attackerKey,
		NodeId:        signerID.String(), // the victim's node
		OwnerId:       "victim",
		EncryptedMeta: []byte("encrypted meta naming the victim"),
	}
	plain := testJPEG(t, 0x40)
	meta, err := claim.Sign(security.ConvertToSHA256(plain))
	require.NoError(t, err)

	forged, err := EmbedInJPEG(plain, meta)
	require.NoError(t, err)

	assert.ErrorIs(t, VerifyImage(forged, signerID.String(), "victim"), ErrForgedMetadata)
	assert.ErrorIs(t, VerifyImage(forged, attackerID.String(), "victim"), ErrForgedMetadata)
}

func TestVerifyImage_TransplantedBlockIsRefused(t *testing.T) {
	watermarked := watermarkedJPEG(t, "alice", 0x40)

	meta, err := extractFromJPEG(watermarked)
	require.NoError(t, err)

	transplanted, err := EmbedInJPEG(testJPEG(t, 0xC0), meta)
	require.NoError(t, err)

	assert.ErrorIs(t, VerifyImage(transplanted, signerID.String(), "alice"), ErrForgedMetadata)
}

func TestStripJPEGExif_IsTheInverseOfEmbedding(t *testing.T) {
	plain := testJPEG(t, 0x40)

	embedded, err := EmbedInJPEG(plain, []byte("some metadata"))
	require.NoError(t, err)

	raw, err := stripJPEGExif(embedded)
	require.NoError(t, err)

	assert.Equal(t, plain, raw, "stripping the EXIF must return the bytes that were signed")
}

func TestStripJPEGExif_RejectsNonJPEG(t *testing.T) {
	_, err := stripJPEGExif([]byte("not a jpeg at all"))
	assert.ErrorIs(t, err, ErrMalformedJPEG)
}

func TestEmbedInJPEG_RejectsNonJPEG(t *testing.T) {
	_, err := EmbedInJPEG([]byte("this is not a jpeg"), []byte("meta"))
	assert.Error(t, err, "a non-JPEG upload must not be parsed as one")

	_, err = EmbedInJPEG(nil, []byte("meta"))
	assert.Error(t, err)
}

func TestEmbedInJPEG_WritesTheDescriptionTag(t *testing.T) {
	embedded, err := EmbedInJPEG(testJPEG(t, 0x40), []byte("payload"))
	require.NoError(t, err)

	meta, err := extractFromJPEG(embedded)
	require.NoError(t, err)
	assert.Equal(t, []byte("payload"), meta)
}

func TestVerifyVideo_HoldsOnlyForTheIdentityItWasWatermarkedFor(t *testing.T) {
	watermarked := watermarkedMP4(t, "alice")

	assert.NoError(t, VerifyVideo(watermarked, signerID.String(), "alice"))
	assert.ErrorIs(t, VerifyVideo(watermarked, signerID.String(), "mallory"), ErrForgedMetadata)

	_, otherNode := mustSigner("another-node")
	assert.ErrorIs(t, VerifyVideo(watermarked, otherNode.String(), "alice"), ErrForgedMetadata)
}

func TestVerifyVideo_UnstampedFileIsRefused(t *testing.T) {
	assert.ErrorIs(t, VerifyVideo(minimalMP4(), signerID.String(), "alice"), ErrNoMetadata)
}

func TestVerifyVideo_TamperedPayloadIsRefused(t *testing.T) {
	watermarked := watermarkedMP4(t, "alice")

	tampered := make([]byte, len(watermarked)+1)
	copy(tampered, watermarked[:24])
	tampered[24] = 'X'
	copy(tampered[25:], watermarked[24:])
	binary.BigEndian.PutUint32(tampered[0:4], 25)

	assert.ErrorIs(t, VerifyVideo(tampered, signerID.String(), "alice"), ErrForgedMetadata)
}

func TestEmbedInVideo_AppendsOneUUIDBox(t *testing.T) {
	watermarked := watermarkedMP4(t, "alice")

	boxes, err := walkBoxes(watermarked)
	require.NoError(t, err)

	var metaBoxes int
	for _, box := range boxes {
		if isWarpnetBox(box) {
			metaBoxes++
		}
	}
	assert.Equal(t, 1, metaBoxes, "exactly one claim of responsibility per file")
	assert.True(t, bytes.HasPrefix(watermarked, minimalMP4()), "the video itself is untouched")
}

func TestSplitVideo_SeparatesWhatTheSignatureCovers(t *testing.T) {
	watermarked := watermarkedMP4(t, "alice")

	raw, watermarkBytes, err := SplitVideo(watermarked)
	require.NoError(t, err)
	assert.Equal(t, minimalMP4(), raw)

	var signed signedWatermark
	require.NoError(t, json.Unmarshal(watermarkBytes, &signed))
	assert.NotEmpty(t, signed.Signature)
}

func TestSplitVideo_RefusesTwoMetaBoxes(t *testing.T) {
	watermarked := watermarkedMP4(t, "alice")

	boxes, err := walkBoxes(watermarked)
	require.NoError(t, err)

	var metaBox []byte
	for _, box := range boxes {
		if isWarpnetBox(box) {
			metaBox = watermarked[box.offset : box.offset+box.size]
		}
	}
	require.NotNil(t, metaBox)

	_, _, err = SplitVideo(append(watermarked, metaBox...))
	assert.ErrorIs(t, err, ErrAmbiguousMetadata)
}

func TestSplitVideo_UnreadableBoxPayloadIsMissingMeta(t *testing.T) {
	header := make([]byte, boxHeaderSize)
	binary.BigEndian.PutUint32(header, uint32(boxHeaderSize+boxUUIDSize+4))
	copy(header[4:], uuidBoxType)

	file := append(minimalMP4(), header...)
	file = append(file, warpnetUUID[:]...)
	file = append(file, []byte("!!!!")...) // not base64

	_, _, err := SplitVideo(file)
	assert.ErrorIs(t, err, ErrNoMetadata)
}

func TestCloseOpenEndedBox(t *testing.T) {
	openEnded := append(minimalMP4(), []byte{
		0x00, 0x00, 0x00, 0x00, // size 0: to the end of the file
		'm', 'd', 'a', 't',
		0xAA, 0xBB, 0xCC, 0xDD,
	}...)

	closed, err := CloseOpenEndedBox(openEnded)
	require.NoError(t, err)
	assert.Equal(t, uint32(12), binary.BigEndian.Uint32(closed[24:28]))

	meta, err := watermark("alice").Sign(security.ConvertToSHA256(closed))
	require.NoError(t, err)
	watermarked, err := EmbedInVideo(closed, meta)
	require.NoError(t, err)

	assert.NoError(t, VerifyVideo(watermarked, signerID.String(), "alice"))
}

func TestWalkBoxes_RejectsMalformed(t *testing.T) {
	_, err := walkBoxes([]byte{0x00, 0x00, 0xFF, 0xFF, 'f', 't', 'y', 'p'})
	assert.ErrorIs(t, err, ErrMalformedISO)
}

func TestIsISOBaseMedia(t *testing.T) {
	leadingFree := append([]byte{
		0x00, 0x00, 0x00, 0x08, 'f', 'r', 'e', 'e',
	}, minimalMP4()...)

	assert.True(t, IsISOBaseMediaFile(minimalMP4()))
	assert.True(t, IsISOBaseMediaFile(leadingFree))
	assert.True(t, IsISOBaseMediaFile(watermarkedMP4(t, "alice")), "a watermarked file is still a container")
	assert.False(t, IsISOBaseMediaFile([]byte{0x1A, 0x45, 0xDF, 0xA3, 0, 0, 0, 0}))
	assert.False(t, IsISOBaseMediaFile([]byte("short")))
	assert.False(t, IsISOBaseMediaFile(nil))
	assert.False(t, IsISOBaseMediaFile([]byte{0x00, 0x00, 0x00, 0x00, 'f', 'r', 'e', 'e'}))
}

func TestEmbedInVideo_RoundTripsThroughBase64(t *testing.T) {
	watermarked := watermarkedMP4(t, "alice")

	idx := bytes.Index(watermarked, warpnetUUID[:])
	require.GreaterOrEqual(t, idx, 0, "warpnet meta uuid box not found")

	decoded, err := base64.StdEncoding.DecodeString(string(watermarked[idx+len(warpnetUUID):]))
	require.NoError(t, err)

	var signed signedWatermark
	assert.NoError(t, json.Unmarshal(decoded, &signed))
}

func TestVerifyImage_StrippedWatermarkIsRefused(t *testing.T) {
	watermarked := watermarkedJPEG(t, "alice", 0x40)
	require.NoError(t, VerifyImage(watermarked, signerID.String(), "alice"))

	stripped, err := stripJPEGExif(watermarked)
	require.NoError(t, err)

	assert.ErrorIs(t, VerifyImage(stripped, signerID.String(), "alice"), ErrNoMetadata)
}

func TestVerifyVideo_StrippedWatermarkIsRefused(t *testing.T) {
	watermarked := watermarkedMP4(t, "alice")
	require.NoError(t, VerifyVideo(watermarked, signerID.String(), "alice"))

	raw, _, err := SplitVideo(watermarked)
	require.NoError(t, err)

	assert.ErrorIs(t, VerifyVideo(raw, signerID.String(), "alice"), ErrNoMetadata)
}

func TestVerify_TamperedSignatureIsRefused(t *testing.T) {
	rawJPEG := testJPEG(t, 0x40)
	hash := security.ConvertToSHA256(rawJPEG)

	watermarkBytes, err := watermark("alice").Sign(hash)
	require.NoError(t, err)

	var signed signedWatermark
	require.NoError(t, json.Unmarshal(watermarkBytes, &signed))

	flipped := []byte(signed.Signature)
	flipped[0] ^= 'A' ^ 'B'
	signed.Signature = string(flipped)

	tampered, err := json.Marshal(signed)
	require.NoError(t, err)

	assert.ErrorIs(t, verify(tampered, hash, signerID.String(), "alice"), ErrForgedMetadata)
}

func TestVerify_UnknownVersionIsRefused(t *testing.T) {
	rawJPEG := testJPEG(t, 0x40)
	hash := security.ConvertToSHA256(rawJPEG)

	watermarkBytes, err := watermark("alice").Sign(hash)
	require.NoError(t, err)

	var signed signedWatermark
	require.NoError(t, json.Unmarshal(watermarkBytes, &signed))
	signed.Version = metaVersion + 1

	future, err := json.Marshal(signed)
	require.NoError(t, err)

	assert.ErrorIs(t, verify(future, hash, signerID.String(), "alice"), ErrNoMetadata)
}

func TestVerify_SignatureWithoutEncryptedMetaIsRefused(t *testing.T) {
	rawJPEG := testJPEG(t, 0x40)
	hash := security.ConvertToSHA256(rawJPEG)

	empty := Watermark{PrivKey: signerKey, NodeId: signerID.String(), OwnerId: "alice"}
	watermarkBytes, err := empty.Sign(hash)
	require.NoError(t, err)

	assert.ErrorIs(t, verify(watermarkBytes, hash, signerID.String(), "alice"), ErrNoMetadata)
}

func TestVerifyImage_GarbageInDescriptionTagIsRefused(t *testing.T) {
	for name, payload := range map[string][]byte{
		"not JSON":   []byte("just some text a camera wrote"),
		"empty JSON": []byte("{}"),
	} {
		t.Run(name, func(t *testing.T) {
			file, err := EmbedInJPEG(testJPEG(t, 0x40), payload)
			require.NoError(t, err)

			assert.ErrorIs(t, VerifyImage(file, signerID.String(), "alice"), ErrNoMetadata)
		})
	}
}
