package tracing

import (
	"firestige.xyz/otus/internal/log"

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
	submita8888       func(*v1.SniffData)
}

func NewTraceListener(serviceName, serviceInstanceId string, submit func(*v1.SniffData)) *TraceListener {
	return &TraceListener{
		serviceName:       serviceName,
		serviceInstanceId: serviceInstanceId,
		manager:           NewTraceManager(serviceName, serviceInstanceId),
		submit:            submit,
	}
}

// 此处处理事件依赖 session_handler正确的事件发布顺序，即严格按照先创建 Dialog 再处理transaction 最后结束 Dialog的顺序
func (l *TraceListener) Init() {
	event.Subscribe(dialog.EventCreateDialog, l.onDialogCreated)
	event.Subscribe(dialog.EventTerminateDialog, l.onDialogTerminated)
	event.Subscribe(transaction.EventCreateTransaction, l.onTransactionCreated)
	event.Subscribe(transaction.EventTerminateTransaction, l.onTransactionTerminated)
	event.Subscribe(transaction.EventTransactionTimeout, l.onTransactionTimeout)
}

func (l *TraceListener) onDialogCreated(ex *processor.Exchange) {
	dialog, ok := data.(types.Dialog)
	if !ok {
		log.GetLogger().Errorf("invalid dialog created event data: %v", data)
		return
	}
	ctx, exist := l.manager.GetTraceContextByCallID(dialog.CallID())
	if !exist {
		log.GetLogger().Errorf("trace context not found for dialog with Call-ID: %s", dialog.CallID())
		return
	}
	ctx.addNewSpanWithDialog(dialog)
}

func (l *TraceListener) onDialogTerminated(ex *processor.Exchange) {
	dialog, ok := data.(types.Dialog)
	if !ok {
		log.GetLogger().Errorf("invalid dialog terminated event data: %v", data)
		return
	}
	ctx, exist := l.manager.GetTraceContextByCallID(dialog.CallID())
	if !exist {
		log.GetLogger().Errorf("trace context not found for dialog with Call-ID: %s", dialog.CallID())
		return
	}
	ctx.endSpanWithDialog(dialog)

	// 通过 ex 把消息发送出去
	ex.CopyWith(nil).Submit()
}

func (l *TraceListener) onTransactionCreated(data interface{}) {
	transaction, ok := data.(types.Transaction)
	if !ok {
		log.GetLogger().Errorf("invalid transaction created event data: %v", data)
		return
	}
	ctx, exist := l.manager.GetTraceContextByCallID(transaction.Request().CallID())
	if !exist {
		log.GetLogger().Errorf("trace context not found for transaction with Call-ID: %s", transaction.Request().CallID())
		return
	}
	ctx.addNewSpanWithTransaction(transaction)
}

func (l *TraceListener) onTransactionTerminated(data interface{}) {
	transaction, ok := data.(types.Transaction)
	if !ok {
		log.GetLogger().Errorf("invalid transaction timeout event data: %v", data)
		return
	}
	ctx, exist := l.manager.GetTraceContextByCallID(transaction.Request().CallID())
	if !exist {
		log.GetLogger().Errorf("trace context not found for transaction with Call-ID: %s", transaction.Request().CallID())
		return
	}
	ctx.endSpanWithTransaction(transaction)
}

func (l *TraceListener) onTransactionTimeout(data interface{}) {
	transaction, ok := data.(types.Transaction)
	if !ok {
		log.GetLogger().Errorf("invalid transaction timeout event data: %v", data)
		return
	}
	ctx, exist := l.manager.GetTraceContextByCallID(transaction.Request().CallID())
	if !exist {
		log.GetLogger().Errorf("trace context not found for transaction with Call-ID: %s", transaction.Request().CallID())
		return
	}
	ctx.endSpanWithTimeout(transaction)
}

// func (l *TraceListener) OnRequest(req types.SipRequest, ua types.UAType) {
// 	l.initContext(req, ua)
// }

// func (l *TraceListener) initContext(req types.SipRequest, uaType types.UAType) {
// 	switch req.Method() {
// 	// TODO 只处理特定的SIP方法，后续应该做成可配置的
// 	case types.MethodInvite, types.MethodRegister, types.MethodOptions:
// 		// 首先根据提取TraceID的策略处理请求
// 		traceID := GetTraceIDFromRequest(req)
// 		ctx, exists := l.manager.GetTraceContextByTraceID(traceID)
// 		if exists {
// 			// 首个请求不应该有TraceContext，需要处理异常
// 			err := fmt.Errorf("trace context already exists for trace ID %s", traceID)
// 			log.GetLogger().WithError(err).Errorf("failed to inital segment: %s", traceID)
// 			return
// 		}
// 		ctx = l.manager.CreateTraceContext(traceID, req.CreatedAt())
// 		l.manager.AliasWithCallID(traceID, req.CallID())
// 		ctx.initSegmentObject(req, uaType)
// 	}
// }

// // TODO 根据实际情况修改
// func GetTraceIDFromRequest(req types.SipRequest) string {
// 	if traceId, exist := req.Headers()[string(types.HeaderNameX_ICC_CALL_ID)]; !exist {
// 		return req.CallID()
// 	} else {
// 		return traceId // 假设只有一个值
// 	}
// }

// func (l *TraceListener) OnDialogCreated(dialog types.Dialog) {
// 	ctx, exist := l.manager.GetTraceContextByCallID(dialog.CallID())
// 	if !exist {
// 		log.GetLogger().Errorf("trace context not found for dialog with Call-ID: %s", dialog.CallID())
// 		return
// 	}
// 	ctx.CreateNewSpan(dialog.ID(), dialog.CallID(), string(types.MethodInvite), dialog.RemoteURI(), dialog.CreatedAt(), dialog.Metadatas())
// }

// func (l *TraceListener) OnDialogStateChanged(dialog types.Dialog) {
// 	// 一般dialog状态变化时不需要更新Segment和Span
// }

// func (l *TraceListener) OnDialogTerminated(dialog types.Dialog) {
// 	log.GetLogger().Debugf("Dialog terminated: %s", dialog.ID())
// 	ctx, exist := l.manager.GetTraceContextByCallID(dialog.CallID())
// 	if !exist {
// 		log.GetLogger().Errorf("trace context not found for dialog with Call-ID: %s", dialog.CallID())
// 		return
// 	}
// 	ctx.FinishExistSpan(dialog.ID(), false, dialog.UpdatedAt()) // 我们认为事务有成功与失败，会话没有
// 	data := sniffdata.WrapWithSniffData(ctx.segment)            // 发送Segment
// 	l.submit(data)                                              // 提交Segment到输出通道
// 	// l.manager.RemoveTraceContextByCallID(dialog.CallID()) // TODO会话结束后移除
// }

// func (l *TraceListener) OnTransactionCreated(transaction types.Transaction) {
// 	ctx, exist := l.manager.GetTraceContextByCallID(transaction.Request().CallID())
// 	if !exist {
// 		log.GetLogger().Errorf("trace context not found for dialog with Call-ID: %s", transaction.Request().CallID())
// 		return
// 	}
// 	// 创建新的Span
// 	callID := transaction.Request().CallID()
// 	method := transaction.Request().MethodAsString()
// 	startTime := transaction.CreatedAt()
// 	headers := transaction.Request().Headers()
// 	switch transaction.UA() {
// 	case types.UAClient:
// 		remoteURI, _ := utils.ExtractURIAndTag(transaction.Request().To())
// 		ctx.CreateNewSpan(transaction.ID(), callID, method, remoteURI, startTime, headers)
// 	case types.UAServer:
// 		// 对于服务器端请求，使用From作为对端地址
// 		remoteURI, _ := utils.ExtractURIAndTag(transaction.Request().From())
// 		ctx.CreateNewSpan(transaction.ID(), callID, method, remoteURI, startTime, headers)
// 	}
// }

// func (l *TraceListener) OnTransactionStateChanged(transaction types.Transaction) {
// 	// 一般dialog状态变化时不需要更新Segment和Span
// }

// func (l *TraceListener) OnTransactionTerminated(tx types.Transaction) {
// 	log.GetLogger().Debugf("Transaction terminated: %s", tx.ID())
// 	ctx, exist := l.manager.GetTraceContextByCallID(tx.Request().CallID())
// 	if !exist {
// 		log.GetLogger().Errorf("trace context not found for dialog with Call-ID: %s", tx.Request().CallID())
// 		return
// 	}
// 	isError := tx.LastResponse() != nil && tx.LastResponse().Status() >= 300
// 	// 结束现有的Span
// 	ctx.FinishExistSpan(tx.ID(), isError, tx.UpdatedAt())
// 	switch tx.Request().Method() {
// 	case types.MethodInvite, types.MethodInfo, types.MethodBye, types.MethodCancel:
// 		// 对于这些方法，我们不需要发送Segment,由dialog生命周期发送
// 		return
// 	default:
// 		data := sniffdata.WrapWithSniffData(ctx.segment) // 发送Segment
// 		l.submit(data)                                   // 提交Segment到输出通道
// 	}
// }

// func (l *TraceListener) OnTransactionTimeout(tx types.Transaction) {
// 	ctx, exist := l.manager.GetTraceContextByCallID(tx.Request().CallID())
// 	if !exist {
// 		log.GetLogger().Errorf("trace context not found for dialog with Call-ID: %s", tx.Request().CallID())
// 		return
// 	}
// 	ctx.FinishExistSpan(tx.ID(), true, tx.UpdatedAt()) // 超时场景一定是错误
// 	switch tx.Request().Method() {
// 	case types.MethodInfo, types.MethodBye, types.MethodCancel:
// 		// 对于这些方法，我们不需要发送Segment
// 		return
// 	default:
// 		data := sniffdata.WrapWithSniffData(ctx.segment) // 发送Segment
// 		l.submit(data)                                   // 提交Segment到输出通道
// 	}
// }

// func (l *TraceListener) OnTransactionError(tx types.Transaction, err error) {
// 	ctx, exist := l.manager.GetTraceContextByCallID(tx.Request().CallID())
// 	if !exist {
// 		log.GetLogger().Errorf("trace context not found for dialog with Call-ID: %s", tx.Request().CallID())
// 		return
// 	}
// 	ctx.FinishExistSpan(tx.ID(), true, tx.UpdatedAt()) // 会话错误场景一定是错误
// 	switch tx.Request().Method() {
// 	case types.MethodInfo, types.MethodBye, types.MethodCancel:
// 		// 对于这些方法，我们不需要发送Segment
// 		return
// 	default:
// 		data := sniffdata.WrapWithSniffData(ctx.segment) // 发送Segment
// 		l.submit(data)                                   // 提交Segment到输出通道
// 	}
// }
