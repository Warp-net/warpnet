// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package srtp

import (
	"io"

	"github.com/pion/transport/v5/packetio"
)

type attributeReader interface {
	ReadWithAttributes([]byte, packetio.Attributes) (int, packetio.Attributes, error)
}

type attributeWriter interface {
	WriteWithAttributes([]byte, packetio.Attributes) (int, error)
}

func readWithAttributes(reader io.Reader, buf []byte, attrs packetio.Attributes) (int, packetio.Attributes, error) {
	if reader, ok := reader.(attributeReader); ok {
		return reader.ReadWithAttributes(buf, attrs)
	}
	clear(attrs)
	n, err := reader.Read(buf)

	return n, attrs[:0], err
}

func writeWithAttributes(writer io.Writer, buf []byte, attrs packetio.Attributes) (int, error) {
	if writer, ok := writer.(attributeWriter); ok {
		return writer.WriteWithAttributes(buf, attrs)
	}

	return writer.Write(buf)
}

type readStream interface {
	init(child streamSession, ssrc uint32) error

	Read(buf []byte) (int, error)
	GetSSRC() uint32
}
