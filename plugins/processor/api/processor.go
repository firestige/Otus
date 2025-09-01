package api

import (
	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/plugin"
)

type Processor interface {
	plugin.Plugin
	Process(packet *otus.NetPacket) error
	SetInputChannel(partition int, ch <-chan *otus.NetPacket) error
	SetOutputChannel(partition int, ch chan<- *otus.OutputPacketContext) error
	IsChannelSet(partition int) bool
}
