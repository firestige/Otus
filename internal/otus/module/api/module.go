package api

import (
	"time"

	otus "firestige.xyz/otus/internal/otus/api"
	"github.com/google/gopacket"
)

var ShutdownHookTime = time.Second * 5

type Module interface {
	PostConstruct() error
	Boot()
	Shutdown()
}

type DataSource interface {
	Boot()
	Stop()
	ReadPacket() ([]byte, gopacket.CaptureInfo, error)
}

type Processor interface {
	Module
}

type Sender interface {
	Module
	Send(*otus.Exchange)
}
