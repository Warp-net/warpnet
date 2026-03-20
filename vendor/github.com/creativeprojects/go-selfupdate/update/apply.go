package update

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/creativeprojects/go-selfupdate/internal"
)

var (
	openFile = os.OpenFile
)

// Apply performs an update of the current executable (or opts.TargetFile, if set) with the contents of the given io.Reader.
//
// Apply performs the following actions to ensure a safe cross-platform update:
//
// 1. If configured, computes the checksum of the new executable and verifies it matches.
//
// 2. If configured, verifies the signature with a public key.
//
// 3. Creates a new file, /path/to/.target.new with the TargetMode with the contents of the updated file
//
// 4. Renames /path/to/target to /path/to/.target.old
//
// 5. Renames /path/to/.target.new to /path/to/target
//
// 6. If the final rename is successful, deletes /path/to/.target.old, returns no error. On Windows,
// the removal of /path/to/target.old always fails, so instead Apply hides the old file instead.
//
// 7. If the final rename fails, attempts to roll back by renaming /path/to/.target.old
// back to /path/to/target.
//
// If the roll back operation fails, the file system is left in an inconsistent state (between steps 5 and 6) where
// there is no new executable file and the old executable file could not be moved to its original location. In this
// case you should notify the user of the bad news and ask them to recover manually. Applications can determine whether
// the rollback failed by calling RollbackError, see the documentation on that function for additional detail.
func Apply(update io.Reader, opts Options) error {
	// validate
	verify := false
	switch {
	case opts.Signature != nil && opts.PublicKey != nil:
		// okay
		verify = true
	case opts.Signature != nil:
		return errors.New("no public key to verify signature with")
	case opts.PublicKey != nil:
		return errors.New("no signature to verify with")
	}

	// set defaults
	if opts.Hash == 0 {
		opts.Hash = crypto.SHA256
	}
	if opts.Verifier == nil {
		opts.Verifier = NewECDSAVerifier()
	}
	if opts.TargetMode == 0 {
		opts.TargetMode = 0o755
	}

	// get target path
	var err error
	if opts.TargetPath == "" {
		opts.TargetPath, err = internal.GetExecutablePath()
		if err != nil {
			return err
		}
	}

	var newBytes []byte
	if newBytes, err = io.ReadAll(update); err != nil {
		return err
	}

	// verify checksum if requested
	if opts.Checksum != nil {
		if err = opts.verifyChecksum(newBytes); err != nil {
			return err
		}
	}

	if verify {
		if err = opts.verifySignature(newBytes); err != nil {
			return err
		}
	}

	// get the directory the executable exists in
	updateDir := filepath.Dir(opts.TargetPath)
	filename := filepath.Base(opts.TargetPath)

	// Copy the contents of newbinary to a new executable file
	newPath := filepath.Join(updateDir, fmt.Sprintf(".%s.new", filename))
	fp, err := openFile(newPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, opts.TargetMode)
	if err != nil {
		return err
	}
	defer fp.Close()

	_, err = io.Copy(fp, bytes.NewReader(newBytes))
	if err != nil {
		return err
	}

	// if we don't call fp.Close(), windows won't let us move the new executable
	// because the file will still be "in use"
	fp.Close()

	// this is where we'll move the executable to so that we can swap in the updated replacement
	oldPath := opts.OldSavePath
	removeOld := opts.OldSavePath == ""
	if removeOld {
		oldPath = filepath.Join(updateDir, fmt.Sprintf(".%s.old", filename))
	}

	// delete any existing old exec file - this is necessary on Windows for two reasons:
	// 1. after a successful update, Windows can't remove the .old file because the process is still running
	// 2. windows rename operations fail if the destination file already exists
	_ = os.Remove(oldPath)

	// move the existing executable to a new file in the same directory
	err = os.Rename(opts.TargetPath, oldPath)
	if err != nil {
		return err
	}

	// move the new executable in to become the new program
	err = os.Rename(newPath, opts.TargetPath)

	if err != nil {
		// move unsuccessful
		//
		// The filesystem is now in a bad state. We have successfully
		// moved the existing binary to a new location, but we couldn't move the new
		// binary to take its place. That means there is no file where the current executable binary
		// used to be!
		// Try to rollback by restoring the old binary to its original path.
		rerr := os.Rename(oldPath, opts.TargetPath)
		if rerr != nil {
			return &rollbackError{err, rerr}
		}

		return err
	}

	// move successful, remove the old binary if needed
	if removeOld {
		errRemove := os.Remove(oldPath)

		// windows has trouble with removing old binaries, so hide it instead
		if errRemove != nil {
			_ = hideFile(oldPath)
		}
	}

	return nil
}

// RollbackError takes an error value returned by Apply and returns the error, if any,
// that occurred when attempting to roll back from a failed update. Applications should
// always call this function on any non-nil errors returned by Apply.
//
// If no rollback was needed or if the rollback was successful, RollbackError returns nil,
// otherwise it returns the error encountered when trying to roll back.
func RollbackError(err error) error {
	if err == nil {
		return nil
	}
	if rerr, ok := err.(*rollbackError); ok {
		return rerr.rollbackErr
	}
	return nil
}

type rollbackError struct {
	error             // original error
	rollbackErr error // error encountered while rolling back
}
