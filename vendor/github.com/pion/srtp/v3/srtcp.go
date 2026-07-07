// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package srtp

import (
	"encoding/binary"
	"fmt"

	"github.com/pion/rtcp"
)

/*
Simplified structure of SRTCP Packets:
- RTCP Header
- Payload
- AEAD Auth Tag - used by AEAD profiles only
- E flag and SRTCP Index
- MKI (optional)
- Auth Tag - used by non-AEAD profiles only
*/

const (
	maxSRTCPIndex = 0x7FFFFFFF

	srtcpHeaderSize     = 8
	srtcpIndexSize      = 4
	srtcpEncryptionFlag = 0x80
)

func (c *Context) decryptRTCP(dst, encrypted []byte) ([]byte, error) {
	authTagLen, err := c.cipher.AuthTagRTCPLen()
	if err != nil {
		return nil, err
	}
	aeadAuthTagLen, err := c.cipher.AEADAuthTagLen()
	if err != nil {
		return nil, err
	}
	mkiLen := len(c.sendMKI)

	// Verify that encrypted packet is long enough
	if len(encrypted) < (srtcpHeaderSize + aeadAuthTagLen + srtcpIndexSize + mkiLen + authTagLen) {
		return nil, fmt.Errorf("%w: %d", errTooShortRTCP, len(encrypted))
	}

	index := c.cipher.getRTCPIndex(encrypted)
	ssrc := binary.BigEndian.Uint32(encrypted[4:])

	// The SSRC is read from the unauthenticated RTCP header at this point.
	// getSRTCPSSRCState is called in read-only mode so that no new map entry is
	// inserted until after the auth tag has been verified. The state is committed
	// to the map by setSRTCPSSRCState only after markAsValid() succeeds below.
	ssrcState, existingState := c.getSRTCPSSRCState(ssrc, false)

	// The replay check is intentionally performed before authentication.
	// Rejecting already-seen sequence numbers here avoids the CPU cost of
	// AES decryption and HMAC/GCM verification on flooded duplicate packets.
	// Safety relies on the replay detector only committing the index as "seen"
	// when markAsValid() is explicitly called after successful authentication.
	markAsValid, ok := ssrcState.replayDetector.Check(uint64(index))
	if !ok {
		return nil, &duplicatedError{Proto: "srtcp", SSRC: ssrc, Index: index}
	}

	cipher := c.cipher
	if len(c.mkis) > 0 {
		// Find cipher for MKI
		actualMKI := encrypted[len(encrypted)-mkiLen-authTagLen : len(encrypted)-authTagLen]
		cipher, ok = c.mkis[string(actualMKI)]
		if !ok {
			return nil, ErrMKINotFound
		}
	}

	out, err := cipher.decryptRTCP(dst, encrypted, index, ssrc)
	if err != nil {
		return nil, err
	}

	markAsValid()

	if !existingState {
		c.setSRTCPSSRCState(ssrcState)
	}

	return out, nil
}

// DecryptRTCP decrypts a buffer that contains a RTCP packet.
func (c *Context) DecryptRTCP(dst, encrypted []byte, header *rtcp.Header) ([]byte, error) {
	if header == nil {
		header = &rtcp.Header{}
	}

	if err := header.Unmarshal(encrypted); err != nil {
		return nil, err
	}

	return c.decryptRTCP(dst, encrypted)
}

func (c *Context) encryptRTCP(dst, decrypted []byte) ([]byte, error) {
	if len(decrypted) < srtcpHeaderSize {
		return nil, fmt.Errorf("%w: %d", errTooShortRTCP, len(decrypted))
	}

	ssrc := binary.BigEndian.Uint32(decrypted[4:])
	ssrcState, _ := c.getSRTCPSSRCState(ssrc, true)

	if ssrcState.srtcpIndex >= maxSRTCPIndex {
		// ... when 2^48 SRTP packets or 2^31 SRTCP packets have been secured with the same key
		// (whichever occurs before), the key management MUST be called to provide new master key(s)
		// (previously stored and used keys MUST NOT be used again), or the session MUST be terminated.
		// https://www.rfc-editor.org/rfc/rfc3711#section-9.2
		return nil, errExceededMaxPackets
	}

	// We roll over early because MSB is used for marking as encrypted
	ssrcState.srtcpIndex++

	return c.cipher.encryptRTCP(dst, decrypted, ssrcState.srtcpIndex, ssrc)
}

// EncryptRTCP Encrypts a RTCP packet.
func (c *Context) EncryptRTCP(dst, decrypted []byte, header *rtcp.Header) ([]byte, error) {
	if header == nil {
		header = &rtcp.Header{}
	}

	if err := header.Unmarshal(decrypted); err != nil {
		return nil, err
	}

	return c.encryptRTCP(dst, decrypted)
}
