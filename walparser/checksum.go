package walparser

import (
	"encoding/binary"
	"fmt"
)

// ChecksumValidator validates WAL frame checksums
type ChecksumValidator struct {
	bigEndian bool
}

// NewChecksumValidator creates a new checksum validator
func NewChecksumValidator(bigEndian bool) *ChecksumValidator {
	return &ChecksumValidator{
		bigEndian: bigEndian,
	}
}

// ValidateFrame validates a frame's checksum
func (cv *ChecksumValidator) ValidateFrame(frame *WALFrame, header *WALHeader, frameData []byte) error {
	var order binary.ByteOrder
	if cv.bigEndian {
		order = binary.BigEndian
	} else {
		order = binary.LittleEndian
	}

	// Calculate expected checksum
	expectedChecksum1, expectedChecksum2 := calculateChecksum(
		frameData,
		header.Checksum1,
		header.Checksum2,
		order,
	)

	// Compare with frame's checksum
	if expectedChecksum1 != frame.Checksum1 || expectedChecksum2 != frame.Checksum2 {
		return fmt.Errorf("%w: expected (%d, %d), got (%d, %d)",
			ErrWALCorrupted,
			expectedChecksum1, expectedChecksum2,
			frame.Checksum1, frame.Checksum2)
	}

	return nil
}

// ValidateHeader validates the WAL header checksum
func (cv *ChecksumValidator) ValidateHeader(header *WALHeader, headerData []byte) error {
	var order binary.ByteOrder
	if cv.bigEndian {
		order = binary.BigEndian
	} else {
		order = binary.LittleEndian
	}

	// Header checksum covers first 24 bytes
	expectedChecksum1, expectedChecksum2 := calculateChecksum(
		headerData[0:24],
		0,
		0,
		order,
	)

	if expectedChecksum1 != header.Checksum1 || expectedChecksum2 != header.Checksum2 {
		return fmt.Errorf("%w: header checksum mismatch", ErrWALCorrupted)
	}

	return nil
}

// DetectCorruption checks for WAL corruption by validating checksums
func DetectCorruption(frame *WALFrame, header *WALHeader, bigEndian bool) error {
	validator := NewChecksumValidator(bigEndian)

	// Build frame data for checksum validation
	// Frame checksum includes: page number (4) + db size (4) + payload
	frameData := make([]byte, 8+len(frame.Payload))

	var order binary.ByteOrder
	if bigEndian {
		order = binary.BigEndian
	} else {
		order = binary.LittleEndian
	}

	order.PutUint32(frameData[0:4], frame.PageNumber)
	order.PutUint32(frameData[4:8], frame.DBSize)
	copy(frameData[8:], frame.Payload)

	return validator.ValidateFrame(frame, header, frameData)
}
