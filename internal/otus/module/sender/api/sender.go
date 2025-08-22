package api

import (
	otus "firestige.xyz/otus/internal/otus/api"
	module "firestige.xyz/otus/internal/otus/module/api"
)

type Sender interface {
	module.Module

	InputNetPacketChannel(partition int) chan<- *otus.OutputPacketContext
	SetCapture(c module.Module) error
}
