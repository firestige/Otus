package pipeline

import (
	otus "firestige.xyz/otus/internal/otus/api"
)

type packetHandler struct {
}

func newPacketHandler() *packetHandler {
	// todo: initialize handleFunc based on cfg
	return &packetHandler{}
}

func (h *packetHandler) handle(exchange *otus.Exchange) {

}
