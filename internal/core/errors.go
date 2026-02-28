// Package core defines sentinel errors.
package core

import "errors"

// Sentinel errors following ADR-021 error handling pattern.
var (
	// Task management errors
	ErrTaskNotFound      = errors.New("capture-agent: task not found")
	ErrTaskAlreadyExists = errors.New("capture-agent: task already exists")
	ErrTaskStartFailed   = errors.New("capture-agent: task start failed")

	// Pipeline errors
	ErrPipelineStopped = errors.New("capture-agent: pipeline stopped")

	// Packet decoding errors
	ErrPacketTooShort   = errors.New("capture-agent: packet too short")
	ErrUnsupportedProto = errors.New("capture-agent: unsupported protocol")

	// IP reassembly errors
	ErrReassemblyTimeout  = errors.New("capture-agent: fragment reassembly timeout")
	ErrReassemblyLimit    = errors.New("capture-agent: fragment reassembly limit exceeded")
	ErrFragmentIncomplete = errors.New("capture-agent: fragment not complete")

	// Plugin errors
	ErrPluginNotFound   = errors.New("capture-agent: plugin not found")
	ErrPluginInitFailed = errors.New("capture-agent: plugin init failed")

	// Configuration errors
	ErrConfigInvalid = errors.New("capture-agent: invalid configuration")

	// Daemon errors
	ErrDaemonNotRunning = errors.New("capture-agent: daemon not running")
)
