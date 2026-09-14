//go:build !android && !ios && ((linux && amd64) || (linux && arm64) || (darwin && amd64) || (darwin && arm64) || (windows && amd64))

package paymentengine

import (
	"bytes"
	"compress/gzip"
	"io"
)

func GetPaymentEngine() ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()

	return io.ReadAll(reader)
}
