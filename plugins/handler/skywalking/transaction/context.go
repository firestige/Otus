package transaction

import (
	"time"

	"firestige.xyz/otus/internal/eventbus"
	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/plugins/filter/skywalking/types"
	"firestige.xyz/otus/plugins/filter/skywalking/utils"
)

type TransactionContext struct {
	state        TransactionState
	id           string
	txType       types.TransactionType
	ua           types.UAType
	req          types.SipRequest
	lastResponse types.SipResponse
	createAt     int64                     // 创建时间
	updatedAt    int64                     // 更新时间
	err          error                     // 错误信息
	timerMap     map[TimerName]*time.Timer // 定时器映射
	eventBus     eventbus.EventBus
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
		timerMap:  make(map[TimerName]*time.Timer),
	}
}

func (ctx *TransactionContext) HandleMessage(msg types.SipMessage) error {
	err := ctx.state.handleMessage(ctx, msg)
	if err != nil {
		return err
	}
	return nil
}

func (ctx *TransactionContext) transitionTo(newState TransactionState) {
	currentStateName := ctx.state.Name()
	newStateName := newState.Name()
	ctx.state.exit(ctx)
	ctx.state = newState
	newState.enter(ctx)
	if currentStateName != newStateName {
		ctx.updatedAt = utils.CurrentTimeMillis() // 更新更新时间
	}
	log.GetLogger().WithField("Transaction-ID", ctx.ID()).Infof("Transitioned transaction state, from %s to %s", currentStateName, newStateName)
}

func (ctx *TransactionContext) createTransactionEvent() *eventbus.Event {
	return nil
}

func (ctx *TransactionContext) StartTimer(name TimerName, span TimerSpan) {
	timer := time.AfterFunc(time.Duration(span)*time.Millisecond, func() {
		ctx.cancelTimer(name)
		ctx.timeout()
	})
	ctx.timerMap[name] = timer
}

func (ctx *TransactionContext) cancelTimer(name TimerName) {
	if timer, exists := ctx.timerMap[name]; exists {
		timer.Stop()
		delete(ctx.timerMap, name)
	}
}

func (ctx *TransactionContext) resetTimer(name TimerName) {
	if timer, exists := ctx.timerMap[name]; exists {
		timer.Reset(time.Duration(timer.C) * time.Millisecond)
	}
}

func (ctx *TransactionContext) timeout() {
	ctx.state.transmitTimeout(ctx)
	ctx.cancelTimer(TimerB)
	ctx.cancelTimer(TimerF)
	ctx.cancelTimer(TimerH)
	ctx.cancelTimer(TimerI)
	ctx.cancelTimer(TimerJ)
	ctx.cancelTimer(TimerK)
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
