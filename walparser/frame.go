package walparser

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	WALHeaderSize = 32 // WAL file header is 32 bytes
	FrameHeaderSize = 24 // Frame header is 24 bytes (excluding payload)

	// Magic numbers for byte order detection
	WALMagicBigEndian    = 0x377f0683
	WALMagicLittleEndian = 0x377f0682
)

// ReadWALHeader reads and parses the 32-byte WAL file header
func ReadWALHeader(r io.Reader) (*WALHeader, error) {
	buf := make([]byte, WALHeaderSize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("failed to read WAL header: %w", err)
	}

	// Read magic number to determine byte order
	magic := binary.BigEndian.Uint32(buf[0:4])

	var header WALHeader
	var order binary.ByteOrder

	if magic == WALMagicBigEndian {
		header.BigEndian = true
		order = binary.BigEndian
	} else if magic == WALMagicLittleEndian {
		header.BigEndian = false
		order = binary.LittleEndian
		// Re-read magic with correct byte order
		magic = binary.LittleEndian.Uint32(buf[0:4])
	} else {
		return nil, fmt.Errorf("invalid WAL magic number: 0x%x", magic)
	}

	header.Magic = magic
	header.FileFormat = order.Uint32(buf[4:8])
	header.PageSize = order.Uint32(buf[8:12])
	header.CheckpointSeq = order.Uint32(buf[12:16])
	header.Salt1 = order.Uint32(buf[16:20])
	header.Salt2 = order.Uint32(buf[20:24])
	header.Checksum1 = order.Uint32(buf[24:28])
	header.Checksum2 = order.Uint32(buf[28:32])

	// Validate header checksum
	calcChecksum1, calcChecksum2 := calculateChecksum(buf[0:24], 0, 0, order)
	if calcChecksum1 != header.Checksum1 || calcChecksum2 != header.Checksum2 {
		return nil, ErrWALCorrupted
	}

	return &header, nil
}

// ReadFrame reads a single WAL frame (header + payload)
func ReadFrame(r io.Reader, pageSize uint32, header *WALHeader) (*WALFrame, error) {
	// Read frame header (24 bytes)
	headerBuf := make([]byte, FrameHeaderSize)
	if _, err := io.ReadFull(r, headerBuf); err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("failed to read frame header: %w", err)
	}

	var order binary.ByteOrder
	if header.BigEndian {
		order = binary.BigEndian
	} else {
		order = binary.LittleEndian
	}

	frame := &WALFrame{
		PageNumber: order.Uint32(headerBuf[0:4]),
		DBSize:     order.Uint32(headerBuf[4:8]),
		Salt1:      order.Uint32(headerBuf[8:12]),
		Salt2:      order.Uint32(headerBuf[12:16]),
		Checksum1:  order.Uint32(headerBuf[16:20]),
		Checksum2:  order.Uint32(headerBuf[20:24]),
	}

	// Verify salt matches header
	if frame.Salt1 != header.Salt1 || frame.Salt2 != header.Salt2 {
		return nil, fmt.Errorf("frame salt mismatch (frame: %d/%d, header: %d/%d)",
			frame.Salt1, frame.Salt2, header.Salt1, header.Salt2)
	}

	// Detect commit frame (DBSize > 0)
	frame.IsCommit = frame.DBSize > 0

	// Read payload (page data)
	frame.Payload = make([]byte, pageSize)
	if _, err := io.ReadFull(r, frame.Payload); err != nil {
		return nil, fmt.Errorf("failed to read frame payload: %w", err)
	}

	// Validate frame checksum
	// Checksum includes frame header + payload
	frameBuf := append(headerBuf[0:8], frame.Payload...)
	calcChecksum1, calcChecksum2 := calculateChecksum(frameBuf, header.Checksum1, header.Checksum2, order)

	if calcChecksum1 != frame.Checksum1 || calcChecksum2 != frame.Checksum2 {
		return nil, ErrWALCorrupted
	}

	return frame, nil
}

// calculateChecksum implements SQLite's Fibonacci-weighted checksum algorithm
// Algorithm: s0 += x(i) + s1; s1 += x(i+1) + s0
func calculateChecksum(data []byte, s0, s1 uint32, order binary.ByteOrder) (uint32, uint32) {
	// Process data as 32-bit integers
	for i := 0; i < len(data); i += 8 {
		if i+8 > len(data) {
			break
		}

		x0 := order.Uint32(data[i : i+4])
		x1 := order.Uint32(data[i+4 : i+8])

		s0 += x0 + s1
		s1 += x1 + s0
	}

	return s0, s1
}

// FrameSize returns the total size of a frame (header + payload)
func FrameSize(pageSize uint32) int {
	return FrameHeaderSize + int(pageSize)
}
