// Package isonet frames and supervises the TCP connection to the issuer (docs/03 §1, §7.1).
package isonet

import (
	"encoding/binary"
	"fmt"
	"io"
)

const maxFrameSize = 4096 // docs/03 §1

// WriteFrame writes a 2-byte big-endian length header followed by payload.
func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) > maxFrameSize {
		return fmt.Errorf("frame too large: %d bytes (max %d)", len(payload), maxFrameSize)
	}
	header := make([]byte, 2)
	binary.BigEndian.PutUint16(header, uint16(len(payload))) //nolint:gosec // bounded by maxFrameSize check above
	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("write frame header: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("write frame payload: %w", err)
	}
	return nil
}

// ReadFrame reads one length-prefixed frame.
func ReadFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("read frame header: %w", err)
	}
	size := binary.BigEndian.Uint16(header)
	if size > maxFrameSize {
		return nil, fmt.Errorf("frame too large: %d bytes (max %d)", size, maxFrameSize)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("read frame payload: %w", err)
	}
	return payload, nil
}
