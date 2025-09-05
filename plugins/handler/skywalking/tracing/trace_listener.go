package tracing

import (
	"firestige.xyz/otus/internal/log"
	otus "firestige.xyz/otus/internal/otus/event"
	processor "firestige.xyz/otus/internal/otus/module/processor/api"
	"firestige.xyz/otus/plugins/handler/skywalking/dialog"
	"firestige.xyz/otus/plugins/handler/skywalking/event"
	"firestige.xyz/otus/plugins/handler/skywalking/transaction"
	"firestige.xyz/otus/plugins/handler/skywalking/types"
	v1 "skywalking.apache.org/repo/goapi/satellite/data/v1"
)

type TraceListener struct {
	serviceName       string
	serviceInstanceId string
	manager           *TraceManager
}

func NewTraceListener(serviceName, serviceInstanceId string, submit func(*v1.SniffData)) *TraceListener {
	return &TraceListener{
		serviceName:       serviceName,
		serviceInstanceId: serviceInstanceId,
		manager:           NewTraceManager(serviceName, serviceInstanceId),
	}
}

// 此处处理事件依赖 session_handler正确的事件发布顺序，即严格按照先创建 Dialog 再处理transaction 最后结束 Dialog的顺序
func (l *TraceListener) Init() {
	event.Subscribe(string(dialog.EventDialogCreated), l.onDialogCreated)
	event.Subscribe(string(dialog.EventDialogTerminated), l.onDialogTerminated)
	event.Subscribe(string(dialog.EventDialogTimeout), l.onDialogTimeout)
	event.Subscribe(string(transaction.EventTransactionCreated), l.onTransactionCreated)
	event.Subscribe(string(transaction.EventTransactionTerminated), l.onTransactionTerminated)
	event.Subscribe(string(transaction.EventTransactionTimeout), l.onTransactionTimeout)
}

func (l *TraceListener) onDialogCreated(ex *processor.Exchange) {
	e := ex.GetEvent()
	dialog, err := otus.GetValue[types.Dialog](e, "dialog")
	if err != nil {
		log.GetLogger().Errorf("invalid dialog created event data: %v", ex.GetEvent())
		return
	}
	ctx, exist := l.manager.GetTraceContextByRefID(dialog.ID())
	if !exist {
		log.GetLogger().Errorf("trace context not found for dialog with ID: %s", dialog.ID())
		return
	}
	ctx.addNewSpanWithDialog(dialog)
}

func (l *TraceListener) onDialogTerminated(ex *processor.Exchange) {
	e := ex.GetEvent()
	dialog, err := otus.GetValue[types.Dialog](e, "dialog")
	if err != nil {
		log.GetLogger().Errorf("invalid dialog terminated event data: %v", ex.GetEvent())
		return
	}
	ctx, exist := l.manager.GetTraceContextByRefID(dialog.ID())
	if !exist {
		log.GetLogger().Errorf("trace context not found for dialog with ID: %s", dialog.ID())
		return
	}
	ctx.endSpanWithDialog(dialog)
	otus.SetValue(e, "trace_context", ctx.segment)
	// 通过 ex 把消息发送出去
	ex.CopyWith(e).Submit()
}

func (l *TraceListener) onDialogTimeout(ex *processor.Exchange) {
	e := ex.GetEvent()
	dialog, err := otus.GetValue[types.Dialog](e, "dialog")
	if err != nil {
		log.GetLogger().Errorf("invalid dialog timeout event data: %v", ex.GetEvent())
		return
	}
	ctx, exist := l.manager.GetTraceContextByRefID(dialog.ID())
	if !exist {
		log.GetLogger().Errorf("trace context not found for dialog with ID: %s", dialog.ID())
		return
	}
	ctx.endSpanWithDialogTimeout(dialog)
	otus.SetValue(e, "trace_context", ctx.segment)
	// 通过 ex 把消息发送出去
	ex.CopyWith(e).Submit()
}

func (l *TraceListener) onTransactionCreated(ex *processor.Exchange) {
	e := ex.GetEvent()
	transaction, err := otus.GetValue[types.Transaction](e, "transaction")
	if err != nil {
		log.GetLogger().Errorf("invalid transaction created event data: %v", e)
		return
	}
	ctx, exist := l.manager.GetTraceContextByRefID(transaction.ID())
	if !exist {
		log.GetLogger().Errorf("trace context not found for transaction with ID: %s", transaction.ID())
		return
	}
	ctx.addNewSpanWithTransaction(transaction)
}

func (l *TraceListener) onTransactionTerminated(ex *processor.Exchange) {
	e := ex.GetEvent()
	transaction, err := otus.GetValue[types.Transaction](e, "transaction")
	if err != nil {
		log.GetLogger().Errorf("invalid transaction terminated event data: %v", e)
		return
	}
	ctx, exist := l.manager.GetTraceContextByRefID(transaction.ID())
	if !exist {
		log.GetLogger().Errorf("trace context not found for transaction with ID: %s", transaction.ID())
		return
	}
	ctx.endSpanWithTransaction(transaction)
	otus.SetValue(e, "trace_context", ctx.segment)
	// 通过 ex 把消息发送出去
	ex.CopyWith(e).Submit()
}

func (l *TraceListener) onTransactionTimeout(ex *processor.Exchange) {
	e := ex.GetEvent()
	transaction, err := otus.GetValue[types.Transaction](e, "transaction")
	if err != nil {
		log.GetLogger().Errorf("invalid transaction timeout event data: %v", e)
		return
	}
	ctx, exist := l.manager.GetTraceContextByRefID(transaction.ID())
	if !exist {
		log.GetLogger().Errorf("trace context not found for transaction with ID: %s", transaction.ID())
		return
	}
	ctx.endSpanWithTransactionTimeout(transaction)
	otus.SetValue(e, "trace_context", ctx.segment)
	// 通过 ex 把消息发送出去
	ex.CopyWith(e).Submit()
}
