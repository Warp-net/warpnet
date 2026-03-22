package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ulikunitz/xz"
)

var (
	fileTypes = []struct {
		ext        string
		decompress func(src io.Reader, cmd, os, arch string) (io.Reader, error)
	}{
		{".zip", unzip},
		{".tar.gz", untar},
		{".tgz", untar},
		{".gzip", gunzip},
		{".gz", gunzip},
		{".tar.xz", untarxz},
		{".xz", unxz},
		{".bz2", unbz2},
	}
	// pattern copied from bottom of the page: https://semver.org/
	semverPattern = `(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?`
)

// DecompressCommand decompresses the given source. Archive and compression format is
// automatically detected from 'url' parameter, which represents the URL of asset,
// or simply a filename (with an extension).
// This returns a reader for the decompressed command given by 'cmd'. '.zip',
// '.tar.gz', '.tar.xz', '.tgz', '.gz', '.bz2' and '.xz' are supported.
//
// These wrapped errors can be returned:
//   - ErrCannotDecompressFile
//   - ErrExecutableNotFoundInArchive
func DecompressCommand(src io.Reader, url, cmd, os, arch string) (io.Reader, error) {
	for _, fileType := range fileTypes {
		if strings.HasSuffix(url, fileType.ext) {
			return fileType.decompress(src, cmd, os, arch)
		}
	}
	log.Print("File is not compressed")
	return src, nil
}

func unzip(src io.Reader, cmd, os, arch string) (io.Reader, error) {
	log.Print("Decompressing zip file")

	// Zip format requires its file size for Decompressing.
	// So we need to read the HTTP response into a buffer at first.
	buf, err := io.ReadAll(src)
	if err != nil {
		return nil, fmt.Errorf("%w zip file: %v", ErrCannotDecompressFile, err)
	}

	r := bytes.NewReader(buf)
	z, err := zip.NewReader(r, r.Size())
	if err != nil {
		return nil, fmt.Errorf("%w zip file: %s", ErrCannotDecompressFile, err)
	}

	for _, file := range z.File {
		_, name := filepath.Split(file.Name)
		if !file.FileInfo().IsDir() && matchExecutableName(cmd, os, arch, name) {
			log.Printf("Executable file %q was found in zip archive", file.Name)
			return file.Open()
		}
	}

	return nil, fmt.Errorf("%w in zip file: %q", ErrExecutableNotFoundInArchive, cmd)
}

func untar(src io.Reader, cmd, os, arch string) (io.Reader, error) {
	log.Print("Decompressing tar.gz file")

	gz, err := gzip.NewReader(src)
	if err != nil {
		return nil, fmt.Errorf("%w tar.gz file: %s", ErrCannotDecompressFile, err)
	}

	return unarchiveTar(gz, cmd, os, arch)
}

func gunzip(src io.Reader, cmd, os, arch string) (io.Reader, error) {
	log.Print("Decompressing gzip file")

	r, err := gzip.NewReader(src)
	if err != nil {
		return nil, fmt.Errorf("%w gzip file: %s", ErrCannotDecompressFile, err)
	}

	name := r.Header.Name
	if !matchExecutableName(cmd, os, arch, name) {
		return nil, fmt.Errorf("%w: expected %q but found %q", ErrExecutableNotFoundInArchive, cmd, name)
	}

	log.Printf("Executable file %q was found in gzip file", name)
	return r, nil
}

func untarxz(src io.Reader, cmd, os, arch string) (io.Reader, error) {
	log.Print("Decompressing tar.xz file")

	xzip, err := xz.NewReader(src)
	if err != nil {
		return nil, fmt.Errorf("%w tar.xz file: %s", ErrCannotDecompressFile, err)
	}

	return unarchiveTar(xzip, cmd, os, arch)
}

func unxz(src io.Reader, cmd, os, arch string) (io.Reader, error) {
	log.Print("Decompressing xzip file")

	xzip, err := xz.NewReader(src)
	if err != nil {
		return nil, fmt.Errorf("%w xzip file: %s", ErrCannotDecompressFile, err)
	}

	log.Printf("Decompressed file from xzip is assumed to be an executable: %s", cmd)
	return xzip, nil
}

func unbz2(src io.Reader, cmd, os, arch string) (io.Reader, error) {
	log.Print("Decompressing bzip2 file")

	bz2 := bzip2.NewReader(src)

	log.Printf("Decompressed file from bzip2 is assumed to be an executable: %s", cmd)
	return bz2, nil
}

func matchExecutableName(cmd, os, arch, target string) bool {
	cmd = strings.TrimSuffix(cmd, ".exe")
	pattern := regexp.MustCompile(
		fmt.Sprintf(
			`^%s([_-]v?%s)?([_-]%s[_-]%s)?(\.exe)?$`,
			regexp.QuoteMeta(cmd),
			semverPattern,
			regexp.QuoteMeta(os),
			regexp.QuoteMeta(arch),
		),
	)
	return pattern.MatchString(target)
}

func unarchiveTar(src io.Reader, cmd, os, arch string) (io.Reader, error) {
	t := tar.NewReader(src)
	for {
		h, err := t.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w tar file: %s", ErrCannotDecompressFile, err)
		}
		_, name := filepath.Split(h.Name)
		if matchExecutableName(cmd, os, arch, name) {
			log.Printf("Executable file %q was found in tar archive", h.Name)
			return t, nil
		}
	}
	return nil, fmt.Errorf("%w in tar: %q", ErrExecutableNotFoundInArchive, cmd)
}
