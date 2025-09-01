package dialog

import (
	"fmt"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/plugins/reporter/skywalkingtracing/types"
	"firestige.xyz/otus/plugins/reporter/skywalkingtracing/utils"
)

type DialogContext struct {
	id        string
	state     DialogState
	ua        types.UAType
	callID    string
	local     string
	remote    string
	createAt  int64
	updatedAt int64
	metaData  map[string]string
}

func NewDialogContext(req types.SipRequest) (*DialogContext, error) {
	// 快速失败，如果不是Invite方法，不应该有Dialog
	if req.Method() != types.MethodInvite {
		return nil, fmt.Errorf("invalid method %s for dialog creation", req.Method())
	}
	// TODO 注意早期对话没有TO头
	id := utils.BuildDialogID(req, true)
	ua := utils.ParseUAType(req)
	callID := req.CallID()
	state := &EarlyState{}
	switch ua {
	case types.UAClient:
		local := req.From()
		remote := req.To()
		return &DialogContext{
			id:        id,
			state:     state,
			ua:        ua,
			callID:    callID,
			local:     local,
			remote:    remote,
			createAt:  req.CreatedAt(),
			updatedAt: req.CreatedAt(),
			metaData:  make(map[string]string),
		}, nil
	case types.UAServer:
		local := req.To()
		remote := req.From()
		return &DialogContext{
			id:        callID,
			state:     state,
			ua:        ua,
			callID:    callID,
			local:     local,
			remote:    remote,
			createAt:  req.CreatedAt(),
			updatedAt: req.CreatedAt(),
			metaData:  make(map[string]string),
		}, nil
	}
	return nil, fmt.Errorf("unsupported UA type %v for dialog creation", ua)
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

func (ctx *DialogContext) State() DialogState {
	// 返回当前对话的状态
	return ctx.state
}

func (ctx *DialogContext) CallID() string {
	// 返回对话的 Call-ID
	return ctx.callID
}

func (ctx *DialogContext) CreatedAt() int64 {
	// 返回对话的创建时间戳
	return ctx.createAt
}

func (ctx *DialogContext) UpdatedAt() int64 {
	// 返回对话的最后更新时间戳
	return ctx.updatedAt
}

func (ctx *DialogContext) UA() types.UAType {
	// 返回对话的用户代理类型
	return ctx.ua
}

func (ctx *DialogContext) LocalURI() string {
	// 返回本地 URI
	uri, _ := utils.ExtractURIAndTag(ctx.local)
	return uri
}

func (ctx *DialogContext) LocalTag() string {
	// 返回本地标签
	_, tag := utils.ExtractURIAndTag(ctx.local)
	return tag
}

func (ctx *DialogContext) RemoteURI() string {
	// 返回远程 URI
	uri, _ := utils.ExtractURIAndTag(ctx.remote)
	return uri
}

func (ctx *DialogContext) RemoteTag() string {
	// 返回远程标签
	_, tag := utils.ExtractURIAndTag(ctx.remote)
	return tag
}

func (ctx *DialogContext) Metadatas() map[string]string {
	return ctx.metaData
}
