package dialog

import (
	"fmt"

	"firestige.xyz/otus/plugins/handler/skywalking/types"
	"firestige.xyz/otus/plugins/handler/skywalking/utils"
)

type DialogState interface {
	Name() string
	IsTerminated() bool // 是否为终止状态
	HandleMessage(ctx *DialogContext, msg types.SipMessage) (DialogState, error)
	Enter(ctx *DialogContext)
	Exit(ctx *DialogContext)
}

type EarlyState struct{}

func (s *EarlyState) Name() string {
	return "EarlyState"
}

func (s *EarlyState) IsTerminated() bool {
	return false
}

func (s *EarlyState) Enter(ctx *DialogContext) {
	// 初始化对话状态，什么都不做
}

func (s *EarlyState) Exit(ctx *DialogContext) {
	// 清理对话状态
}

func (s *EarlyState) HandleMessage(ctx *DialogContext, msg types.SipMessage) (DialogState, error) {
	if resp, ok := msg.(types.SipResponse); ok {
		if utils.IsProvisionalResponse(resp) {
			// 1xx响应，保持Early
			return s, nil
		}
		if utils.Is2XXResponse(resp) {
			// 2xx响应，进入Confirmed
			next := &ConfirmedState{}
			return next, nil
		}
		if utils.IsNon2XXFinalResponse(resp) {
			// 非2xx最终响应，进入Terminated
			next := &TerminatedState{}
			return next, nil
		}
		// 其他响应，忽略
		return s, nil
	}
	if req, ok := msg.(types.SipRequest); ok {
		switch req.Method() {
		case types.MethodBye, types.MethodCancel:
			// BYE或CANCEL请求，进入Terminated
			next := &TerminatedState{}
			return next, nil
		default:
			// 其他请求，保持Early
			return s, nil
		}
	}
	return s, fmt.Errorf("EarlyState: unhandled message type %T", msg)
}

type ConfirmedState struct{}

func (s *ConfirmedState) Name() string {
	return "ConfirmedState"
}

func (s *ConfirmedState) IsTerminated() bool {
	return false
}

func (s *ConfirmedState) Enter(ctx *DialogContext) {
	// 初始化已确认状态
}

func (s *ConfirmedState) HandleMessage(ctx *DialogContext, msg types.SipMessage) (DialogState, error) {
	if req, ok := msg.(types.SipRequest); ok {
		switch req.Method() {
		case types.MethodBye, types.MethodCancel:
			// BYE或CANCEL请求，进入Terminated
			next := &TerminatedState{}
			return next, nil
		default:
			// 其他请求，保持Confirmed
			return s, fmt.Errorf("ConfirmedState: unhandled request method %s", req.Method())
		}
	}
	return s, fmt.Errorf("ConfirmedState: unhandled message type %T", msg)
}

func (s *ConfirmedState) Exit(ctx *DialogContext) {
	// 清理已确认状态
}

type TerminatedState struct{}

func (s *TerminatedState) Name() string {
	return "TerminatedState"
}

func (s *TerminatedState) IsTerminated() bool {
	return true
}

func (s *TerminatedState) Enter(ctx *DialogContext) {
	// 初始化终止状态
}

func (s *TerminatedState) HandleMessage(ctx *DialogContext, msg types.SipMessage) (DialogState, error) {
	// 终止态不再处理任何事件
	return nil, fmt.Errorf("dialog already terminated")
}

func (s *TerminatedState) Exit(ctx *DialogContext) {
	// 清理终止状态
}
