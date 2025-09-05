package transaction

import (
	"firestige.xyz/otus/plugins/handler/skywalking/types"
)

type Handler struct {
}

func (h *Handler) Handle(sipMsg types.SipMessage) error {
	return nil
}
