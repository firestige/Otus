package pipeline

import (
	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/config"
)

type packetHandler struct {
}

func newPacketHandler(cfg *config.OtusConfig) *packetHandler {
	// todo: initialize handleFunc based on cfg
	return &packetHandler{}
}

func (h *packetHandler) handle(exchange *otus.Exchange) {

}
