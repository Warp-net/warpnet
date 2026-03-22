package selfupdate

import "errors"

// Possible errors returned
var (
	ErrNotSupported                = errors.New("operation not supported")
	ErrInvalidSlug                 = errors.New("invalid slug format, expected 'owner/name'")
	ErrIncorrectParameterOwner     = errors.New("incorrect parameter \"owner\"")
	ErrIncorrectParameterRepo      = errors.New("incorrect parameter \"repo\"")
	ErrInvalidID                   = errors.New("invalid repository ID, expected 'owner/name' but found number")
	ErrInvalidRelease              = errors.New("invalid release (nil argument)")
	ErrAssetNotFound               = errors.New("asset not found")
	ErrValidationAssetNotFound     = errors.New("validation file not found")
	ErrValidatorNotFound           = errors.New("file did not match a configured validator")
	ErrIncorrectChecksumFile       = errors.New("incorrect checksum file format")
	ErrChecksumValidationFailed    = errors.New("sha256 validation failed")
	ErrHashNotFound                = errors.New("hash not found in checksum file")
	ErrECDSAValidationFailed       = errors.New("ECDSA signature verification failed")
	ErrInvalidECDSASignature       = errors.New("invalid ECDSA signature")
	ErrInvalidPGPSignature         = errors.New("invalid PGP signature")
	ErrPGPKeyRingNotSet            = errors.New("PGP key ring not set")
	ErrCannotDecompressFile        = errors.New("failed to decompress")
	ErrExecutableNotFoundInArchive = errors.New("executable not found")
)
