package api

import (
	"firestige.xyz/otus/internal/otus/api"
	module "firestige.xyz/otus/internal/otus/module/api"
)

type Sender interface {
	module.Module

	InputNetPacketChannel() chan<- *api.OutputPacketContext
	SetCapture(c module.Module) error
}
