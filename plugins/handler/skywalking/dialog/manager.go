package dialog

import (
	"sync"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/plugins/filter/skywalking/types"
	"firestige.xyz/otus/plugins/filter/skywalking/utils"
)

type DialogManager struct {
	store     *sync.Map // 使用 dialog-ID 作为对话标识
	listeners []types.DialogListener
}

func NewDialogManager() *DialogManager {
	return &DialogManager{
		store:     &sync.Map{},
		listeners: make([]types.DialogListener, 0),
	}
}

func (dm *DialogManager) CreateDialog(req types.SipRequest) *DialogContext {
	ctx, err := NewDialogContext(req)
	if err != nil {
		log.GetLogger().WithError(err).Errorf("Failed to create dialog for request: %s", req.StartLine())
		return nil // 如果创建对话失败，返回 nil
	}
	dm.store.Store(ctx.ID(), ctx)
	for _, listener := range dm.listeners {
		listener.OnDialogCreated(ctx)
	}
	return ctx
}

func (dm *DialogManager) GetDialogByID(id string) (*DialogContext, bool) {
	ctx, exists := dm.store.Load(id)
	if !exists {
		return nil, false // 如果对话不存在，返回 nil 和 false
	}
	return ctx.(*DialogContext), true
}

func (dm *DialogManager) GetAllDialogs() []*DialogContext {
	var allDialogs []*DialogContext
	dm.store.Range(func(key, value interface{}) bool {
		allDialogs = append(allDialogs, value.(*DialogContext))
		return true
	})
	return allDialogs
}

func (dm *DialogManager) GetDialogBySipMessage(msg types.SipMessage) (*DialogContext, bool) {
	// TODO 先不考虑fork场景，简化模型，统一到早期对话
	dialogID := utils.BuildDialogID(msg, true)
	ctx, exists := dm.store.Load(dialogID)
	if !exists {
		dialogID := utils.BuildDialogID(msg, true)
		ctx, exists = dm.store.Load(dialogID)
		if !exists {
			return nil, false // 如果对话不存在，返回 nil 和 false
		}
	}
	return ctx.(*DialogContext), true // 返回找到的对话上下文和 true
}

func (dm *DialogManager) HandleMessage(msg types.SipMessage) error {
	dx, exist := dm.GetDialogBySipMessage(msg)
	if !exist && msg.IsRequest() {
		if req, ok := msg.(types.SipRequest); ok {
			dx = dm.CreateDialog(req)
			if dx != nil {
				log.GetLogger().Infof("Created new dialog: %s", dx.ID())
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
			for _, listener := range dm.listeners {
				listener.OnDialogTerminated(dx)
			}
		}
	}
	return err
}
