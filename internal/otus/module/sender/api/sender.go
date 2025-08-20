package api

import (
	"firestige.xyz/otus/internal/otus/module/api"
	"firestige.xyz/otus/internal/otus/msg"
)

type Sender interface {
	api.Module

	InputNetPacketChannel() chan<- *msg.OutputMessage
	SetCapture(c api.Module) error
}
