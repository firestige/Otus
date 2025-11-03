package afpacket

import (
	"fmt"
)

// recomputeSize recalculates the frame size, block size, and number of blocks
// to meet Linux AF_PACKET PACKET_MMAP mechanism's strict alignment requirements
// while optimizing packet capture performance within the target memory budget.
//
// AF_PACKET PACKET_MMAP requires:
// 1. frameSize must be a multiple of TPACKET_ALIGNMENT (typically 16 bytes)
// 2. blockSize must be a multiple of pageSize (typically 4096 bytes)
// 3. blockSize must be a multiple of frameSize
// 4. Total memory = blockSize * numBlocks should approximate ringBufferSizeMB
//
// Parameters:
//   - ringBufferSizeMB: Target ring buffer size in megabytes
//   - snapLen: Maximum bytes to capture per packet (snapshot length)
//   - pageSize: System page size in bytes (typically 4096)
//
// Returns:
//   - frameSize: Size of each frame in bytes (aligned)
//   - blockSize: Size of each block in bytes (aligned)
//   - numBlocks: Number of blocks to allocate
//   - err: Error if parameters are invalid or cannot be satisfied
func recomputeSize(ringBufferSizeMB, snapLen, pageSize int) (frameSize, blockSize, numBlocks int, err error) {
	const tpacketAlignment = 16 // TPACKET_ALIGNMENT for AF_PACKET
	const tpacketHdrLen = 52    // TPACKET2_HDRLEN or TPACKET3_HDRLEN (approximate)

	// Validate input parameters
	if ringBufferSizeMB <= 0 {
		return 0, 0, 0, fmt.Errorf("ringBufferSizeMB must be positive, got %d", ringBufferSizeMB)
	}
	if snapLen <= 0 {
		return 0, 0, 0, fmt.Errorf("snapLen must be positive, got %d", snapLen)
	}
	if pageSize <= 0 || pageSize%tpacketAlignment != 0 {
		return 0, 0, 0, fmt.Errorf("pageSize must be positive and multiple of %d, got %d", tpacketAlignment, pageSize)
	}

	targetBytes := ringBufferSizeMB * 1024 * 1024

	// Step 1: Calculate frame size (header + packet data), aligned to TPACKET_ALIGNMENT
	rawFrameSize := tpacketHdrLen + snapLen
	frameSize = ((rawFrameSize + tpacketAlignment - 1) / tpacketAlignment) * tpacketAlignment

	// Step 2: Calculate block size as a multiple of both pageSize and frameSize
	// Start with a reasonable block size (e.g., 128 KB or 256 KB)
	minBlockSize := pageSize
	if minBlockSize < frameSize {
		minBlockSize = frameSize
	}

	// Find the LCM (Least Common Multiple) of pageSize and frameSize
	blockSize = lcm(pageSize, frameSize)

	// Ensure blockSize is reasonable (not too small, not too large)
	// Typical block sizes are 4KB to 4MB
	maxBlockSize := 4 * 1024 * 1024 // 4 MB
	if blockSize < minBlockSize {
		blockSize = minBlockSize
	}
	if blockSize > maxBlockSize {
		// If LCM is too large, use a practical block size
		blockSize = maxBlockSize
		// Ensure it's still a multiple of pageSize
		blockSize = (blockSize / pageSize) * pageSize
	}

	// Step 3: Calculate number of blocks
	numBlocks = targetBytes / blockSize
	if numBlocks < 1 {
		numBlocks = 1
	}

	// Validate that blockSize is a multiple of frameSize
	if blockSize%frameSize != 0 {
		// Adjust blockSize to be a multiple of frameSize
		framesPerBlock := blockSize / frameSize
		if framesPerBlock < 1 {
			framesPerBlock = 1
		}
		blockSize = framesPerBlock * frameSize

		// Re-align to pageSize
		blockSize = ((blockSize + pageSize - 1) / pageSize) * pageSize
	}

	return frameSize, blockSize, numBlocks, nil
}

// gcd computes the greatest common divisor of two integers
func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// lcm computes the least common multiple of two integers
func lcm(a, b int) int {
	if a == 0 || b == 0 {
		return 0
	}
	return (a * b) / gcd(a, b)
}
