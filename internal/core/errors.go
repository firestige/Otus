// Package core defines sentinel errors.
package core

import "errors"

// Sentinel errors following ADR-021 error handling pattern.
var (
	// Task management errors
	ErrTaskNotFound      = errors.New("otus: task not found")
	ErrTaskAlreadyExists = errors.New("otus: task already exists")
	ErrTaskStartFailed   = errors.New("otus: task start failed")

	// Pipeline errors
	ErrPipelineStopped = errors.New("otus: pipeline stopped")

	// Packet decoding errors
	ErrPacketTooShort   = errors.New("otus: packet too short")
	ErrUnsupportedProto = errors.New("otus: unsupported protocol")

	// IP reassembly errors
	ErrReassemblyTimeout = errors.New("otus: fragment reassembly timeout")
	ErrReassemblyLimit   = errors.New("otus: fragment reassembly limit exceeded")

	// Plugin errors
	ErrPluginNotFound   = errors.New("otus: plugin not found")
	ErrPluginInitFailed = errors.New("otus: plugin init failed")

	// Configuration errors
	ErrConfigInvalid = errors.New("otus: invalid configuration")

	// Daemon errors
	ErrDaemonNotRunning = errors.New("otus: daemon not running")
)
