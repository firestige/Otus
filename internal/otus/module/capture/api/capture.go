package api

import (
	"firestige.xyz/otus/internal/otus/api"
	module "firestige.xyz/otus/internal/otus/module/api"
)

type Capture interface {
	module.Module

	PartitionCount() int
	OutputPacketChannel(partition int) chan *api.BatchePacket
	SetProcessor(processor module.Module)
}
