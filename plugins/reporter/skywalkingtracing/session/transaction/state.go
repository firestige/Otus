package transaction

import (
	"fmt"

	"firestige.xyz/otus/plugins/reporter/skywalkingtracing/types"
	"firestige.xyz/otus/plugins/reporter/skywalkingtracing/utils"
)

type TransactionState interface {
	Name() string
	IsTerminated() bool // 是否为终止状态
	HandleMessage(ctx *TransactionContext, msg types.SipMessage) (TransactionState, error)
	Enter(ctx *TransactionContext)
	Exit(ctx *TransactionContext)
}

// ---- Non-INVITE Transaction States ----

type NonInviteTryingState struct{}

func (s *NonInviteTryingState) Name() string {
	return "NonInviteTryingState"
}

func (s *NonInviteTryingState) Enter(ctx *TransactionContext) {
	ctx.StartTimer(TimerE, T1)
}

func (s *NonInviteTryingState) HandleMessage(ctx *TransactionContext, msg types.SipMessage) (TransactionState, error) {
	if req, ok := msg.(types.SipRequest); ok {
		if req.Method() == types.MethodInvite {
			return s, nil // 保持Trying状态
		}
	}
	if resp, ok := msg.(types.SipResponse); ok {
		if utils.IsProvisionalResponse(resp) {
			// 1xx响应，进入Proceeding
			ctx.lastResponse = resp
			return &NonInviteProceedingState{}, nil
		}
		if utils.IsFinalResponse(resp) {
			// 最终响应，进入Complete
			ctx.lastResponse = resp
			next := &NonInviteCompletedState{}
			return next, nil
		}
	}
	// 其他响应，保持Trying
	return s, nil
}

func (s *NonInviteTryingState) Exit(ctx *TransactionContext) {

}

func (s *NonInviteTryingState) IsTerminated() bool {
	return false
}

type NonInviteProceedingState struct{}

func (s *NonInviteProceedingState) Name() string {
	return "NonInviteProceedingState"
}

func (s *NonInviteProceedingState) Enter(ctx *TransactionContext) {
	ctx.StartTimer(TimerF, 64*T1)
}

func (s *NonInviteProceedingState) HandleMessage(ctx *TransactionContext, msg types.SipMessage) (TransactionState, error) {
	if resp, ok := msg.(types.SipResponse); ok {
		if utils.IsFinalResponse(resp) {
			// 最终响应，进入Completed
			next := &NonInviteCompletedState{}
			ctx.lastResponse = resp
			return next, nil
		}
	}
	// TODO 注意补齐响应超时事件
	// case EventTimerJ:
	// 	return &NonInviteTerminatedState{}, nil
	return s, nil
}

func (s *NonInviteProceedingState) Exit(ctx *TransactionContext) {

}

func (s *NonInviteProceedingState) IsTerminated() bool {
	return false
}

type NonInviteCompletedState struct{}

func (s *NonInviteCompletedState) Name() string {
	return "NonInviteCompletedState"
}

func (s *NonInviteCompletedState) Enter(ctx *TransactionContext) {
	// 通常在UAS侧启动TimerJ，UAC侧启动TimerK
	// 这里假设都启动TimerJ，具体可根据角色区分
	ctx.StartTimer(TimerJ, 64*T1)
}

func (s *NonInviteCompletedState) HandleMessage(ctx *TransactionContext, msg types.SipMessage) (TransactionState, error) {
	// TODO这里只响应计时器超时事件
	// switch event {
	// case EventTimerJ, EventTimerK:
	// 	return &NonInviteTerminatedState{}, nil
	// default:
	// 	return nil, fmt.Errorf("unexpected event %v in NonInviteCompletedState", event)
	// }
	return nil, fmt.Errorf("unexpected message type %T in NonInviteCompletedState", msg)
}

func (s *NonInviteCompletedState) Exit(ctx *TransactionContext) {
	ctx.CancelTimer(TimerF)
	ctx.CancelTimer(TimerK)
}

func (s *NonInviteCompletedState) IsTerminated() bool {
	return false
}

type NonInviteTerminatedState struct{}

func (s *NonInviteTerminatedState) Name() string {
	return "NonInviteTerminatedState"
}

func (s *NonInviteTerminatedState) Enter(ctx *TransactionContext) {
	// 事务终止，无需操作
}

func (s *NonInviteTerminatedState) HandleMessage(ctx *TransactionContext, msg types.SipMessage) (TransactionState, error) {
	return nil, fmt.Errorf("transaction already terminated")
}

func (s *NonInviteTerminatedState) Exit(ctx *TransactionContext) {}

func (s *NonInviteTerminatedState) IsTerminated() bool {
	return true
}

// ---- INVITE Transaction States ----

type InviteCallingState struct{}

func (s *InviteCallingState) Name() string {
	return "InviteCallingState"
}

func (s *InviteCallingState) Enter(ctx *TransactionContext) {
	ctx.StartTimer(TimerA, T1)
	ctx.StartTimer(TimerB, 64*T1)
}

func (s *InviteCallingState) HandleMessage(ctx *TransactionContext, msg types.SipMessage) (TransactionState, error) {
	if _, ok := msg.(types.SipRequest); ok {
		return s, nil
	}
	if resp, ok := msg.(types.SipResponse); ok {
		if utils.IsProvisionalResponse(resp) {
			// 收到/发出1xx响应，进入Proceeding
			ctx.lastResponse = resp
			return &InviteProceedingState{}, nil
		}
		if utils.Is2XXResponse(resp) {
			// 收到/发出最终响应，进入Terminated
			ctx.lastResponse = resp
			return &InviteTerminatedState{}, nil
		}
		if utils.IsNon2XXFinalResponse(resp) {
			// 收到/发出非2xx最终响应，进入Completed
			ctx.lastResponse = resp
			return &InviteCompletedState{}, nil
		}
	}
	return nil, fmt.Errorf("unexpected message type %T in InviteCallingState", msg)
}

func (s *InviteCallingState) Exit(ctx *TransactionContext) {
	ctx.CancelTimer(TimerA)
	ctx.CancelTimer(TimerB)
}

func (s *InviteCallingState) IsTerminated() bool {
	return false
}

type InviteProceedingState struct{}

func (s *InviteProceedingState) Name() string {
	return "InviteProceedingState"
}

func (s *InviteProceedingState) Enter(ctx *TransactionContext) {
	ctx.StartTimer(TimerC, 64*T1)
}

func (s *InviteProceedingState) HandleMessage(ctx *TransactionContext, msg types.SipMessage) (TransactionState, error) {
	if resp, ok := msg.(types.SipResponse); ok {
		if utils.IsProvisionalResponse(resp) {
			// 收到1xx响应，保持Proceeding
			ctx.lastResponse = resp
			return s, nil
		}
		if utils.Is2XXResponse(resp) {
			// 收到2xx响应，进入Terminated
			ctx.lastResponse = resp
			return &InviteTerminatedState{}, nil
		}
		if utils.IsNon2XXFinalResponse(resp) {
			// 收到非2xx最终响应，进入Terminated
			ctx.lastResponse = resp
			return &InviteCompletedState{}, nil
		}
	}
	return nil, fmt.Errorf("unexpected message type %T in InviteProceedingState", msg)
}

func (s *InviteProceedingState) Exit(ctx *TransactionContext) {
	ctx.CancelTimer(TimerC)
}

func (s *InviteProceedingState) IsTerminated() bool {
	return false
}

type InviteCompletedState struct{}

func (s *InviteCompletedState) Name() string {
	return "InviteCompletedState"
}

func (s *InviteCompletedState) Enter(ctx *TransactionContext) {
	ctx.StartTimer(TimerD, 64*T1)
}

func (s *InviteCompletedState) HandleMessage(ctx *TransactionContext, msg types.SipMessage) (TransactionState, error) {
	if req, ok := msg.(types.SipRequest); ok && req.Method() == types.MethodAck {
		return &InviteConfirmedState{}, nil
	}
	// TODO 注意补齐响应超时事件
	// case EventTimerI:
	// 	return &InviteTerminatedState{}, nil
	return nil, fmt.Errorf("unexpected message type %T in InviteCompletedState", msg)
}

func (s *InviteCompletedState) Exit(ctx *TransactionContext) {
	ctx.CancelTimer(TimerD)
}

func (s *InviteCompletedState) IsTerminated() bool {
	return false
}

type InviteConfirmedState struct{}

func (s *InviteConfirmedState) Name() string {
	return "InviteConfirmedState"
}

func (s *InviteConfirmedState) Enter(ctx *TransactionContext) {
	ctx.StartTimer(TimerI, T1)
}

func (s *InviteConfirmedState) HandleMessage(ctx *TransactionContext, msg types.SipMessage) (TransactionState, error) {
	// TODO这里只响应计时器超时事件
	// switch event {
	// case EventTimerI:
	// 	return &InviteTerminatedState{}, nil
	// default:
	// 	return nil, fmt.Errorf("unexpected event %v in InviteConfirmedState", event)
	// }
	return nil, fmt.Errorf("unexpected message type %T in InviteConfirmedState", msg)
}

func (s *InviteConfirmedState) Exit(ctx *TransactionContext) {
	ctx.CancelTimer(TimerI)
}

func (s *InviteConfirmedState) IsTerminated() bool {
	return false
}

type InviteTerminatedState struct{}

func (s *InviteTerminatedState) Name() string {
	return "InviteTerminatedState"
}

func (s *InviteTerminatedState) IsTerminated() bool {
	return true // 事务已终止
}

func (s *InviteTerminatedState) Enter(ctx *TransactionContext) {
	// 事务终止，无需操作
}

func (s *InviteTerminatedState) HandleMessage(ctx *TransactionContext, msg types.SipMessage) (TransactionState, error) {
	return nil, fmt.Errorf("transaction already terminated")
}

func (s *InviteTerminatedState) Exit(ctx *TransactionContext) {}
