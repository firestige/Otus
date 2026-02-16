// Package core defines core data structures with zero external dependencies.
package core

import "time"

// RawPacket is captured from the network interface.
type RawPacket struct {
Data           []byte
Timestamp      time.Time
CaptureLen     uint32
OrigLen        uint32
InterfaceIndex int
}

// DecodedPacket is the result of L2-L4 protocol stack decoding.
type DecodedPacket struct {
}

// OutputPacket is the final output sent to reporters.
type OutputPacket struct {
}
