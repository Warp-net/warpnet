package selfupdate

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"path"

	"golang.org/x/crypto/openpgp"
)

// Validator represents an interface which enables additional validation of releases.
type Validator interface {
	// Validate validates release bytes against an additional asset bytes.
	// See SHAValidator or ECDSAValidator for more information.
	Validate(filename string, release, asset []byte) error
	// GetValidationAssetName returns the additional asset name containing the validation checksum.
	// The asset containing the checksum can be based on the release asset name
	// Please note if the validation file cannot be found, the DetectLatest and DetectVersion methods
	// will fail with a wrapped ErrValidationAssetNotFound error
	GetValidationAssetName(releaseFilename string) string
}

// RecursiveValidator may be implemented by validators that can continue validation on
// validation assets (multistep validation).
type RecursiveValidator interface {
	// MustContinueValidation returns true if validation must continue on the provided filename
	MustContinueValidation(filename string) bool
}

//=====================================================================================================================

// PatternValidator specifies a validator for additional file validation
// that redirects to other validators depending on glob file patterns.
//
// Unlike others, PatternValidator is a recursive validator that also checks
// validation assets (e.g. SHA256SUMS file checks assets and SHA256SUMS.asc
// checks the SHA256SUMS file).
// Depending on the used validators, a validation loop might be created,
// causing validation errors. In order to prevent this, use SkipValidation
// for validation assets that should not be checked (e.g. signature files).
// Note that glob pattern are matched in the order of addition. Add general
// patterns like "*" at last.
//
// Usage Example (validate assets by SHA256SUMS and SHA256SUMS.asc):
//
//	 new(PatternValidator).
//		// "SHA256SUMS" file is checked by PGP signature (from "SHA256SUMS.asc")
//		Add("SHA256SUMS", new(PGPValidator).WithArmoredKeyRing(key)).
//		// "SHA256SUMS.asc" file is not checked (is the signature for "SHA256SUMS")
//		SkipValidation("*.asc").
//		// All other files are checked by the "SHA256SUMS" file
//		Add("*", &ChecksumValidator{UniqueFilename:"SHA256SUMS"})
type PatternValidator struct {
	validators []struct {
		pattern   string
		validator Validator
	}
}

// Add maps a new validator to the given glob pattern.
func (m *PatternValidator) Add(glob string, validator Validator) *PatternValidator {
	m.validators = append(m.validators, struct {
		pattern   string
		validator Validator
	}{glob, validator})

	if _, err := path.Match(glob, ""); err != nil {
		panic(fmt.Errorf("failed adding %q: %w", glob, err))
	}
	return m
}

// SkipValidation skips validation for the given glob pattern.
func (m *PatternValidator) SkipValidation(glob string) *PatternValidator {
	_ = m.Add(glob, nil)
	// move skip rule to the beginning of the list to ensure it is matched
	// before the validation rules
	if size := len(m.validators); size > 0 {
		m.validators = append(m.validators[size-1:], m.validators[0:size-1]...)
	}
	return m
}

func (m *PatternValidator) findValidator(filename string) (Validator, error) {
	for _, item := range m.validators {
		if match, err := path.Match(item.pattern, filename); match {
			return item.validator, nil
		} else if err != nil {
			return nil, err
		}
	}
	return nil, ErrValidatorNotFound
}

// Validate delegates to the first matching Validator that was configured with Add.
// It fails with ErrValidatorNotFound if no matching validator is configured.
func (m *PatternValidator) Validate(filename string, release, asset []byte) error {
	if validator, err := m.findValidator(filename); err == nil {
		if validator == nil {
			return nil // OK, this file does not need to be validated
		}
		return validator.Validate(filename, release, asset)
	} else {
		return err
	}
}

// GetValidationAssetName returns the asset name for validation.
func (m *PatternValidator) GetValidationAssetName(releaseFilename string) string {
	if validator, err := m.findValidator(releaseFilename); err == nil {
		if validator == nil {
			return releaseFilename // Return a file that we know will exist
		}
		return validator.GetValidationAssetName(releaseFilename)
	} else {
		return releaseFilename // do not produce an error here to ensure err will be logged.
	}
}

// MustContinueValidation returns true if validation must continue on the specified filename
func (m *PatternValidator) MustContinueValidation(filename string) bool {
	if validator, err := m.findValidator(filename); err == nil && validator != nil {
		if rv, ok := validator.(RecursiveValidator); ok && rv != m {
			return rv.MustContinueValidation(filename)
		}
		return true
	}
	return false
}

//=====================================================================================================================

// SHAValidator specifies a SHA256 validator for additional file validation
// before updating.
type SHAValidator struct {
}

// Validate checks the SHA256 sum of the release against the contents of an
// additional asset file.
func (v *SHAValidator) Validate(filename string, release, asset []byte) error {
	// we'd better check the size of the file otherwise it's going to panic
	if len(asset) < sha256.BlockSize {
		return ErrIncorrectChecksumFile
	}

	hash := string(asset[:sha256.BlockSize])
	calculatedHash := fmt.Sprintf("%x", sha256.Sum256(release))

	if equal, err := hexStringEquals(sha256.Size, calculatedHash, hash); !equal {
		if err == nil {
			return fmt.Errorf("expected %q, found %q: %w", hash, calculatedHash, ErrChecksumValidationFailed)
		} else {
			return fmt.Errorf("%s: %w", err.Error(), ErrChecksumValidationFailed)
		}
	}
	return nil
}

// GetValidationAssetName returns the asset name for SHA256 validation.
func (v *SHAValidator) GetValidationAssetName(releaseFilename string) string {
	return releaseFilename + ".sha256"
}

//=====================================================================================================================

// ChecksumValidator is a SHA256 checksum validator where all the validation hash are in a single file (one per line)
type ChecksumValidator struct {
	// UniqueFilename is the name of the global file containing all the checksums
	// Usually "checksums.txt", "SHA256SUMS", etc.
	UniqueFilename string
}

// Validate the SHA256 sum of the release against the contents of an
// additional asset file containing all the checksums (one file per line).
func (v *ChecksumValidator) Validate(filename string, release, asset []byte) error {
	hash, err := findChecksum(filename, asset)
	if err != nil {
		return err
	}
	return new(SHAValidator).Validate(filename, release, []byte(hash))
}

func findChecksum(filename string, content []byte) (string, error) {
	// check if the file has windows line ending (probably better than just testing the platform)
	crlf := []byte("\r\n")
	lf := []byte("\n")
	eol := lf
	if bytes.Contains(content, crlf) {
		log.Print("Checksum file is using windows line ending")
		eol = crlf
	}
	lines := bytes.Split(content, eol)
	log.Printf("Checksum validator: %d checksums available, searching for %q", len(lines), filename)
	for _, line := range lines {
		// skip empty line
		if len(line) == 0 {
			continue
		}
		parts := bytes.Split(line, []byte("  "))
		if len(parts) != 2 {
			return "", ErrIncorrectChecksumFile
		}
		if string(parts[1]) == filename {
			return string(parts[0]), nil
		}
	}
	return "", ErrHashNotFound
}

// GetValidationAssetName returns the unique asset name for SHA256 validation.
func (v *ChecksumValidator) GetValidationAssetName(releaseFilename string) string {
	return v.UniqueFilename
}

//=====================================================================================================================

// ECDSAValidator specifies a ECDSA validator for additional file validation
// before updating.
type ECDSAValidator struct {
	PublicKey *ecdsa.PublicKey
}

// WithPublicKey is a convenience method to set PublicKey from a PEM encoded
// ECDSA certificate
func (v *ECDSAValidator) WithPublicKey(pemData []byte) *ECDSAValidator {
	block, _ := pem.Decode(pemData)
	if block == nil || block.Type != "CERTIFICATE" {
		panic(fmt.Errorf("failed to decode PEM block"))
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err == nil {
		var ok bool
		if v.PublicKey, ok = cert.PublicKey.(*ecdsa.PublicKey); !ok {
			err = fmt.Errorf("not an ECDSA public key")
		}
	}
	if err != nil {
		panic(fmt.Errorf("failed to parse certificate in PEM block: %w", err))
	}

	return v
}

// Validate checks the ECDSA signature of the release against the signature
// contained in an additional asset file.
func (v *ECDSAValidator) Validate(filename string, input, signature []byte) error {
	h := sha256.New()
	h.Write(input)

	log.Printf("Verifying ECDSA signature on %q", filename)
	var rs struct {
		R *big.Int
		S *big.Int
	}
	if _, err := asn1.Unmarshal(signature, &rs); err != nil {
		return ErrInvalidECDSASignature
	}

	if v.PublicKey == nil || !ecdsa.Verify(v.PublicKey, h.Sum([]byte{}), rs.R, rs.S) {
		return ErrECDSAValidationFailed
	}

	return nil
}

// GetValidationAssetName returns the asset name for ECDSA validation.
func (v *ECDSAValidator) GetValidationAssetName(releaseFilename string) string {
	return releaseFilename + ".sig"
}

//=====================================================================================================================

// PGPValidator specifies a PGP validator for additional file validation
// before updating.
type PGPValidator struct {
	// KeyRing is usually filled by openpgp.ReadArmoredKeyRing(bytes.NewReader(key)) with key being the PGP pub key.
	KeyRing openpgp.EntityList
	// Binary toggles whether to validate detached *.sig (binary) or *.asc (ascii) signature files
	Binary bool
}

// WithArmoredKeyRing is a convenience method to set KeyRing
func (g *PGPValidator) WithArmoredKeyRing(key []byte) *PGPValidator {
	if ring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(key)); err == nil {
		g.KeyRing = ring
	} else {
		panic(fmt.Errorf("failed setting armored public key ring: %w", err))
	}
	return g
}

// Validate checks the PGP signature of the release against the signature
// contained in an additional asset file.
func (g *PGPValidator) Validate(filename string, release, signature []byte) (err error) {
	if g.KeyRing == nil {
		return ErrPGPKeyRingNotSet
	}
	log.Printf("Verifying PGP signature on %q", filename)

	data, sig := bytes.NewReader(release), bytes.NewReader(signature)
	if g.Binary {
		_, err = openpgp.CheckDetachedSignature(g.KeyRing, data, sig)
	} else {
		_, err = openpgp.CheckArmoredDetachedSignature(g.KeyRing, data, sig)
	}

	if errors.Is(err, io.EOF) {
		err = ErrInvalidPGPSignature
	}

	return err
}

// GetValidationAssetName returns the asset name for PGP validation.
func (g *PGPValidator) GetValidationAssetName(releaseFilename string) string {
	if g.Binary {
		return releaseFilename + ".sig"
	}
	return releaseFilename + ".asc"
}

//=====================================================================================================================

func hexStringEquals(size int, a, b string) (equal bool, err error) {
	size *= 2
	if len(a) == size && len(b) == size {
		var bytesA, bytesB []byte
		if bytesA, err = hex.DecodeString(a); err == nil {
			if bytesB, err = hex.DecodeString(b); err == nil {
				equal = bytes.Equal(bytesA, bytesB)
			}
		}
	}
	return
}

// NewChecksumWithECDSAValidator returns a validator that checks assets with a checksums file
// (e.g. SHA256SUMS) and the checksums file with an ECDSA signature (e.g. SHA256SUMS.sig).
func NewChecksumWithECDSAValidator(checksumsFilename string, pemECDSACertificate []byte) Validator {
	return new(PatternValidator).
		Add(checksumsFilename, new(ECDSAValidator).WithPublicKey(pemECDSACertificate)).
		Add("*", &ChecksumValidator{UniqueFilename: checksumsFilename}).
		SkipValidation("*.sig")
}

// NewChecksumWithPGPValidator returns a validator that checks assets with a checksums file
// (e.g. SHA256SUMS) and the checksums file with an armored PGP signature (e.g. SHA256SUMS.asc).
func NewChecksumWithPGPValidator(checksumsFilename string, armoredPGPKeyRing []byte) Validator {
	return new(PatternValidator).
		Add(checksumsFilename, new(PGPValidator).WithArmoredKeyRing(armoredPGPKeyRing)).
		Add("*", &ChecksumValidator{UniqueFilename: checksumsFilename}).
		SkipValidation("*.asc")
}

//=====================================================================================================================

// Verify interface
var (
	_ Validator          = &SHAValidator{}
	_ Validator          = &ChecksumValidator{}
	_ Validator          = &ECDSAValidator{}
	_ Validator          = &PGPValidator{}
	_ Validator          = &PatternValidator{}
	_ RecursiveValidator = &PatternValidator{}
)
