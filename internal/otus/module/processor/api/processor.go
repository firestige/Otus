package api

import (
	otus "firestige.xyz/otus/internal/otus/api"
	module "firestige.xyz/otus/internal/otus/module/api"
)

type Processor interface {
	module.Module
	Process(packet *otus.NetPacket)
}
