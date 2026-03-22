package update

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

// Options for Apply update
type Options struct {
	// TargetPath defines the path to the file to update.
	// The empty string means 'the executable file of the running program'.
	TargetPath string

	// Create TargetPath replacement with this file mode. If zero, defaults to 0755.
	TargetMode os.FileMode

	// Checksum of the new binary to verify against. If nil, no checksum or signature verification is done.
	Checksum []byte

	// Public key to use for signature verification. If nil, no signature verification is done.
	PublicKey crypto.PublicKey

	// Signature to verify the updated file. If nil, no signature verification is done.
	Signature []byte

	// Pluggable signature verification algorithm. If nil, ECDSA is used.
	Verifier Verifier

	// Use this hash function to generate the checksum. If not set, SHA256 is used.
	Hash crypto.Hash

	// Store the old executable file at this path after a successful update.
	// The empty string means the old executable file will be removed after the update.
	OldSavePath string
}

// SetPublicKeyPEM is a convenience method to set the PublicKey property
// used for checking a completed update's signature by parsing a
// Public Key formatted as PEM data.
func (o *Options) SetPublicKeyPEM(pembytes []byte) error {
	block, _ := pem.Decode(pembytes)
	if block == nil {
		return errors.New("couldn't parse PEM data")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return err
	}
	o.PublicKey = pub
	return nil
}

func (o *Options) verifyChecksum(updated []byte) error {
	checksum, err := checksumFor(o.Hash, updated)
	if err != nil {
		return err
	}

	if !bytes.Equal(o.Checksum, checksum) {
		return fmt.Errorf("updated file has wrong checksum. Expected: %x, got: %x", o.Checksum, checksum)
	}
	return nil
}

func (o *Options) verifySignature(updated []byte) error {
	checksum, err := checksumFor(o.Hash, updated)
	if err != nil {
		return err
	}
	return o.Verifier.VerifySignature(checksum, o.Signature, o.Hash, o.PublicKey)
}

func checksumFor(h crypto.Hash, payload []byte) ([]byte, error) {
	if !h.Available() {
		return nil, errors.New("requested hash function not available")
	}
	hash := h.New()
	_, _ = hash.Write(payload)
	return hash.Sum([]byte{}), nil
}
