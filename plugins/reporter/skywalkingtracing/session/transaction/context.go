package transaction

import (
	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/plugins/reporter/skywalkingtracing/types"
	"firestige.xyz/otus/plugins/reporter/skywalkingtracing/utils"
)

type TransactionContext struct {
	state        TransactionState
	id           string
	txType       types.TransactionType
	ua           types.UAType
	req          types.SipRequest
	lastResponse types.SipResponse
	createAt     int64                   // 创建时间
	updatedAt    int64                   // 更新时间
	err          error                   // 错误信息
	timerMap     map[TimerName]TimerSpan // 定时器映射
}

func NewTransaction(req types.SipRequest, state TransactionState) *TransactionContext {
	txID := utils.BuildTransactionID(req)
	txType := utils.CreateTransactionType(req)
	ua := utils.ParseUAType(req)
	// 这里可以根据需要返回具体的事务实现
	return &TransactionContext{
		id:        txID,
		state:     state,
		txType:    txType,
		ua:        ua,
		req:       req,
		createAt:  req.CreatedAt(),
		updatedAt: req.CreatedAt(),
		timerMap:  make(map[TimerName]TimerSpan),
	}

}

func (ctx *TransactionContext) HandleMessage(msg types.SipMessage) error {
	newState, err := ctx.state.HandleMessage(ctx, msg)
	if err != nil {
		return err
	}

	ctx.transitionTo(newState)
	return nil
}

func (ctx *TransactionContext) transitionTo(newState TransactionState) {
	currentStateName := ctx.state.Name()
	newStateName := newState.Name()
	ctx.state.Exit(ctx)
	ctx.state = newState
	newState.Enter(ctx)
	if currentStateName != newStateName {
		ctx.updatedAt = utils.CurrentTimeMillis() // 更新更新时间
	}
	log.GetLogger().WithField("Transaction-ID", ctx.ID()).Infof("Transitioned transaction state, from %s to %s", currentStateName, newStateName)
}

func (ctx *TransactionContext) StartTimer(name TimerName, span TimerSpan) {

}

func (ctx *TransactionContext) CancelTimer(name TimerName) {

}

func (ctx *TransactionContext) State() TransactionState {
	return ctx.state
}

func (ctx *TransactionContext) ID() string {
	return ctx.id
}

func (ctx *TransactionContext) Type() types.TransactionType {
	return ctx.txType
}

func (ctx *TransactionContext) UA() types.UAType {
	return ctx.ua
}

func (ctx *TransactionContext) Request() types.SipRequest {
	return ctx.req
}

func (ctx *TransactionContext) LastResponse() types.SipResponse {
	return ctx.lastResponse
}

func (ctx *TransactionContext) CreatedAt() int64 {
	return ctx.createAt
}

func (ctx *TransactionContext) UpdatedAt() int64 {
	return ctx.updatedAt
}

func (ctx *TransactionContext) IsTerminated() bool {
	switch ctx.state.(type) {
	case *InviteTerminatedState, *NonInviteTerminatedState:
		return true
	default:
		return false
	}
}

func (ctx *TransactionContext) Error() error {
	return ctx.err
}
