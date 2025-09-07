package dialog

import (
	"firestige.xyz/otus/internal/log"
	processor "firestige.xyz/otus/internal/otus/module/processor/api"
	"firestige.xyz/otus/plugins/handler/skywalking/types"
)

type Handler struct {
	dm DialogManager
}

func (h *Handler) Handle(ex *processor.Exchange) error {
	dx, exist := h.dm.GetDialogBySipMessage(ex)
	if !exist && ex.IsRequest() {
		if req, ok := ex.(types.SipRequest); ok {
			dx = h.dm.CreateDialog(req)
			if dx != nil {
				for _, listener := range h.listeners {
					listener.OnDialogCreated(dx)
				}
			}
		}
	}
	if dx == nil {
		log.GetLogger().Warnf("No dialog found for message: %s", msg.StartLine())
		return nil // 如果上下文不存在，直接返回
	}
	err := dx.HandleMessage(msg)
	if err == nil {
		if dx.state.IsTerminated() {
			for _, listener := range h.dm.listeners {
				listener.OnDialogTerminated(dx)
			}
		}
	}
	return err
}
