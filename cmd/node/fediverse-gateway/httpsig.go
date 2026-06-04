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

package main

// HTTP Signatures (draft-cavage) for ActivityPub. The crypto and signing-string
// canonicalization are delegated to superseriousbusiness/httpsig — the library
// GoToSocial uses for Mastodon interop. This file keeps only the policy the
// library leaves to the caller: the minimum signed header set, Date freshness
// (replay guard), and binding any request body to a signed SHA-256 digest.

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/superseriousbusiness/httpsig"
)

var (
	errNoSignatureHeader   = errors.New("httpsig: missing Signature header")
	errDigestMismatch      = errors.New("httpsig: digest mismatch")
	errIncompleteSignature = errors.New("httpsig: incomplete Signature header")
	errBadPublicKey        = errors.New("httpsig: bad public key")
	errStaleRequest        = errors.New("httpsig: request date out of range")
)

// draft-cavage signed-header names the gateway emits and requires.
const (
	requestTargetHeader = "(request-target)"
	hostHeader          = "host"
	dateHeader          = "date"
	digestHeader        = "digest"
	headerDate          = "Date" // canonical HTTP header name
)

// minSignedHeaders is the minimum set ActivityPub peers are expected to sign.
var minSignedHeaders = []string{requestTargetHeader, hostHeader, dateHeader}

// maxClockSkew bounds how far a request's Date may deviate from now (replay guard).
const maxClockSkew = 12 * time.Hour

// signRequest signs req in place with keyID/key. For requests with a body,
// pass the already-read body bytes so a Digest header is set and covered.
func signRequest(req *http.Request, keyID string, key *rsa.PrivateKey, body []byte) error {
	if req.Header.Get(headerDate) == "" {
		req.Header.Set(headerDate, time.Now().UTC().Format(http.TimeFormat))
	}
	if req.Host == "" {
		req.Host = req.URL.Host
	}
	// The signer reads the host from req.Header; net/http keeps it in req.Host
	// (and omits this header from the wire), so mirror it for signing.
	req.Header.Set("Host", req.Host)

	headers := []string{requestTargetHeader, hostHeader, dateHeader}
	if body != nil {
		headers = append(headers, digestHeader)
	}

	signer, _, err := httpsig.NewSigner(
		[]httpsig.Algorithm{httpsig.RSA_SHA256},
		httpsig.DigestSha256,
		headers,
		httpsig.Signature,
		0,
	)
	if err != nil {
		return fmt.Errorf("httpsig: new signer: %w", err)
	}
	// The library sets the Digest header from body when "digest" is signed.
	if err := signer.SignRequest(key, keyID, req, body); err != nil {
		return fmt.Errorf("httpsig: sign: %w", err)
	}
	return nil
}

// verifyRequest checks the HTTP signature on an inbound request. body is the
// already-read request body; fetchKey resolves a keyId to its RSA public key
// (by dereferencing the signing actor's document). The library verifies the
// signature itself; the checks below are policy it leaves to the caller.
func verifyRequest(req *http.Request, body []byte, fetchKey func(keyID string) (*rsa.PublicKey, error)) error {
	sigHdr := req.Header.Get("Signature")
	if sigHdr == "" {
		return errNoSignatureHeader
	}
	_, headers, _, err := parseSignatureHeader(sigHdr)
	if err != nil {
		return err
	}

	// Enforce the minimum signed header set required for ActivityPub.
	for _, required := range minSignedHeaders {
		if !slices.Contains(headers, required) {
			return fmt.Errorf("httpsig: %q not signed: %w", required, errIncompleteSignature)
		}
	}
	// Date must be present and recent: signing over an absent/empty date
	// weakens replay protection.
	dateStr := req.Header.Get(headerDate)
	if dateStr == "" {
		return fmt.Errorf("httpsig: missing Date header: %w", errIncompleteSignature)
	}
	when, derr := http.ParseTime(dateStr)
	if derr != nil {
		return fmt.Errorf("httpsig: bad Date header: %w", errIncompleteSignature)
	}
	if skew := time.Since(when); skew > maxClockSkew || skew < -maxClockSkew {
		return errStaleRequest
	}
	// A request carrying a body MUST bind it via a signed digest, otherwise a
	// tampered body would still verify.
	if len(body) > 0 && !slices.Contains(headers, digestHeader) {
		return fmt.Errorf("httpsig: body not bound by digest: %w", errIncompleteSignature)
	}
	if slices.Contains(headers, digestHeader) {
		sum := sha256.Sum256(body)
		want := "SHA-256=" + base64.StdEncoding.EncodeToString(sum[:])
		if req.Header.Get("Digest") != want {
			return errDigestMismatch
		}
	}

	v, err := httpsig.NewVerifier(req)
	if err != nil {
		return fmt.Errorf("httpsig: new verifier: %w", err)
	}
	pub, err := fetchKey(v.KeyId())
	if err != nil {
		return fmt.Errorf("httpsig: fetch key %q: %w", v.KeyId(), err)
	}
	if err := v.Verify(pub, httpsig.RSA_SHA256); err != nil {
		return fmt.Errorf("httpsig: verify: %w", err)
	}
	return nil
}

func parseSignatureHeader(v string) (keyID string, headers []string, signature string, err error) {
	for part := range strings.SplitSeq(v, ",") {
		k, val, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		val = strings.Trim(strings.TrimSpace(val), `"`)
		switch k {
		case "keyId":
			keyID = val
		case "headers":
			headers = strings.Fields(strings.ToLower(val))
		case "signature":
			signature = val
		}
	}
	if keyID == "" || signature == "" {
		return "", nil, "", errIncompleteSignature
	}
	if len(headers) == 0 {
		headers = []string{dateHeader} // draft default
	}
	return keyID, headers, signature, nil
}

func parseRSAPublicKeyPEM(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("not PEM: %w", errBadPublicKey)
	}
	if pub, perr := x509.ParsePKIXPublicKey(block.Bytes); perr == nil {
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("not RSA: %w", errBadPublicKey)
		}
		return rsaPub, nil
	}
	if k, perr := x509.ParsePKCS1PublicKey(block.Bytes); perr == nil {
		return k, nil
	}
	return nil, errBadPublicKey
}
