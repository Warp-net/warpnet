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

// This is a minimal, self-contained implementation of the "Cavage" HTTP
// Signatures draft, enough for the Phase-1 skeleton: the header set
// "(request-target) host date [digest]" with rsa-sha256. The RSA primitives
// are stdlib.
//
// PRODUCTION: replace this file with superseriousbusiness/httpsig (the library
// GoToSocial and Mastodon-compatible servers use) behind signRequest /
// verifyRequest — do not grow a bespoke implementation. Known gaps here:
// no Date-skew check, hs2019 not emitted, no signature reuse/caching.

import (
	"crypto"
	"crypto/rand"
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
)

var (
	errNoSignatureHeader   = errors.New("httpsig: missing Signature header")
	errDigestMismatch      = errors.New("httpsig: digest mismatch")
	errIncompleteSignature = errors.New("httpsig: incomplete Signature header")
	errBadPublicKey        = errors.New("httpsig: bad public key")
	errStaleRequest        = errors.New("httpsig: request date out of range")
)

// requestTargetHeader is the draft-cavage pseudo-header covering method + path.
const requestTargetHeader = "(request-target)"

// minSignedHeaders is the minimum set ActivityPub peers are expected to sign.
var minSignedHeaders = []string{requestTargetHeader, "host", "date"}

// maxClockSkew bounds how far a request's Date may deviate from now (replay guard).
const maxClockSkew = 12 * time.Hour

// signRequest signs req in place with keyID/key. For requests with a body,
// pass the already-read body bytes so a Digest header is set and covered.
func signRequest(req *http.Request, keyID string, key *rsa.PrivateKey, body []byte) error {
	if req.Header.Get("Date") == "" {
		req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	}
	if req.Host == "" {
		req.Host = req.URL.Host
	}

	headers := []string{requestTargetHeader, "host", "date"}
	if body != nil {
		sum := sha256.Sum256(body)
		req.Header.Set("Digest", "SHA-256="+base64.StdEncoding.EncodeToString(sum[:]))
		headers = append(headers, "digest")
	}

	hashed := sha256.Sum256([]byte(buildSigningString(req, headers)))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hashed[:])
	if err != nil {
		return fmt.Errorf("httpsig: sign: %w", err)
	}
	req.Header.Set("Signature", fmt.Sprintf(
		`keyId="%s",algorithm="rsa-sha256",headers="%s",signature="%s"`,
		keyID, strings.Join(headers, " "), base64.StdEncoding.EncodeToString(sig),
	))
	return nil
}

// verifyRequest checks the HTTP signature on an inbound request. body is the
// already-read request body; fetchKey resolves a keyId to its RSA public key
// (by dereferencing the signing actor's document).
func verifyRequest(req *http.Request, body []byte, fetchKey func(keyID string) (*rsa.PublicKey, error)) error {
	sigHdr := req.Header.Get("Signature")
	if sigHdr == "" {
		return errNoSignatureHeader
	}
	keyID, headers, signature, err := parseSignatureHeader(sigHdr)
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
	dateStr := req.Header.Get("Date")
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
	if len(body) > 0 && !slices.Contains(headers, "digest") {
		return fmt.Errorf("httpsig: body not bound by digest: %w", errIncompleteSignature)
	}

	if slices.Contains(headers, "digest") {
		sum := sha256.Sum256(body)
		want := "SHA-256=" + base64.StdEncoding.EncodeToString(sum[:])
		if req.Header.Get("Digest") != want {
			return errDigestMismatch
		}
	}

	pub, err := fetchKey(keyID)
	if err != nil {
		return fmt.Errorf("httpsig: fetch key %q: %w", keyID, err)
	}

	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("httpsig: decode signature: %w", err)
	}
	hashed := sha256.Sum256([]byte(buildSigningString(req, headers)))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, hashed[:], sig); err != nil {
		return fmt.Errorf("httpsig: verify: %w", err)
	}
	return nil
}

// buildSigningString assembles the draft-cavage signing string over the named
// pseudo-headers and real headers, in order.
func buildSigningString(req *http.Request, headers []string) string {
	var b strings.Builder
	for i, h := range headers {
		if i > 0 {
			b.WriteByte('\n')
		}
		switch h {
		case requestTargetHeader:
			fmt.Fprintf(&b, "(request-target): %s %s", strings.ToLower(req.Method), req.URL.RequestURI())
		case "host":
			fmt.Fprintf(&b, "host: %s", req.Host)
		default:
			fmt.Fprintf(&b, "%s: %s", h, req.Header.Get(h))
		}
	}
	return b.String()
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
		headers = []string{"date"} // draft default
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
