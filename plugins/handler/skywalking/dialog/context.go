package dialog

import (
	"fmt"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/plugins/handler/skywalking/types"
	"firestige.xyz/otus/plugins/handler/skywalking/utils"
)

type DialogContext struct {
	id    string
	state DialogState

	metaData map[string]string
}

func NewDialogContext(req types.SipRequest) (*DialogContext, error) {
	// 快速失败，如果不是Invite方法，不应该有Dialog
	if req.Method() != types.MethodInvite {
		return nil, fmt.Errorf("invalid method %s for dialog creation", req.Method())
	}
	id := utils.BuildDialogID(req)
	initialState := &EarlyState{}
	return &DialogContext{
		id:       id,
		state:    initialState,
		metaData: make(map[string]string),
	}, nil
}

func (ctx *DialogContext) HandleMessage(msg types.SipMessage) error {
	newState, err := ctx.state.HandleMessage(ctx, msg)
	if err != nil {
		return err
	}
	ctx.transitionTo(newState)
	return nil
}

func (ctx *DialogContext) transitionTo(newState DialogState) {
	currentStateName := ctx.state.Name()
	newStateName := newState.Name()
	ctx.state.Exit(ctx)
	ctx.state = newState
	newState.Enter(ctx)
	log.GetLogger().WithField("Dialog-ID", ctx.ID()).Infof("Transitioned dialog state, from %s to %s", currentStateName, newStateName)
}

func (ctx *DialogContext) ID() string {
	// 这里可以返回对话的唯一标识，例如 Call-ID
	return ctx.id
}

func (ctx *DialogContext) IsTerminated() bool {
	return ctx.state.IsTerminated()
}

func (ctx *DialogContext) terminate() {
	ctx.transitionTo(&TerminatedState{})
}
