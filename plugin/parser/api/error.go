package api

import "errors"

var (
	ErrIncomplete  = errors.New("segment incomplete")
	ErrNotSIP      = errors.New("not SIP")
	ErrParsePacket = errors.New("failed to parse packet")
)
