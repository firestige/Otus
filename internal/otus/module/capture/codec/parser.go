package codec

import "fmt"

// Package api contains parser contracts used by the parser plugin.
//
// The Parser interface defines how raw byte content is inspected and
// converted into logical messages. Implementations may hold internal
// state and should document concurrency guarantees (for example,
// whether they are safe for concurrent use).

// Parser is the contract for types that can detect and extract
// protocol/application messages from a raw byte stream.
//
// Implementations should clearly document expected behavior for
// partial data (when Extract cannot produce a full message) and for
// error cases.
type Parser interface {
	// Detect inspects the provided content and reports whether this
	// parser recognizes the format and is willing to attempt extraction.
	//
	// Input:
	//   content - raw bytes to test
	// Returns:
	//   true if the parser can handle the content; false otherwise.
	Detect(content []byte) bool

	// Extract attempts to parse one logical message from the beginning
	// of content.
	//
	// Inputs:
	//   content - raw bytes available for parsing
	//
	// Returns:
	//   msg - extracted message bytes (or nil when no complete message)
	//   consumed - number of input bytes consumed from content
	//   err - non-nil when a fatal parsing error occurred
	//
	// Notes:
	//   - If no complete message is available, implementations may
	//     return (nil, 0, nil) or a well-documented error indicating
	//     that more data is required.
	Extract(content []byte) (msg []byte, consumed int, applicationProtocolType string, err error)

	// Reset clears the parser's internal state so it can begin parsing
	// a new independent stream.
	Reset()
}

var (
	ErrNotSIP = fmt.Errorf("not a SIP message")
)
