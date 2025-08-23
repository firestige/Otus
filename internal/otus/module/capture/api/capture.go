package api

import (
	otus "firestige.xyz/otus/internal/otus/api"
	module "firestige.xyz/otus/internal/otus/module/api"
)

type Capture interface {
	module.Module

	PartitionCount() int
	OutputPacketChannel(partition int) <-chan *otus.BatchePacket
	SetOutputChannel(partition int, ch chan<- *otus.BatchePacket) error
}
