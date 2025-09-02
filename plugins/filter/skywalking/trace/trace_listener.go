package trace

import (
	"fmt"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/plugins/filter/skywalking/sniffdata"
	"firestige.xyz/otus/plugins/filter/skywalking/types"
	"firestige.xyz/otus/plugins/filter/skywalking/utils"
	v1 "skywalking.apache.org/repo/goapi/satellite/data/v1"
)

type TraceListener struct {
	serviceName       string
	serviceInstanceId string
	manager           *TraceManager
	submit            func(*v1.SniffData)
}

func NewTraceListener(serviceName, serviceInstanceId string, submit func(*v1.SniffData)) *TraceListener {
	return &TraceListener{
		serviceName:       serviceName,
		serviceInstanceId: serviceInstanceId,
		manager:           NewTraceManager(serviceName, serviceInstanceId),
		submit:            submit,
	}
}

func (l *TraceListener) OnRequest(req types.SipRequest, ua types.UAType) {
	l.initContext(req, ua)
}

func (l *TraceListener) initContext(req types.SipRequest, uaType types.UAType) {
	switch req.Method() {
	// TODO 只处理特定的SIP方法，后续应该做成可配置的
	case types.MethodInvite, types.MethodRegister, types.MethodOptions:
		// 首先根据提取TraceID的策略处理请求
		traceID := GetTraceIDFromRequest(req)
		ctx, exists := l.manager.GetTraceContextByTraceID(traceID)
		if exists {
			// 首个请求不应该有TraceContext，需要处理异常
			err := fmt.Errorf("trace context already exists for trace ID %s", traceID)
			log.GetLogger().WithError(err).Errorf("failed to inital segment: %s", traceID)
			return
		}
		ctx = l.manager.CreateTraceContext(traceID, req.CreatedAt())
		l.manager.AliasWithCallID(traceID, req.CallID())
		ctx.initSegmentObject(req, uaType)
	}
}

// TODO 根据实际情况修改
func GetTraceIDFromRequest(req types.SipRequest) string {
	if traceId, exist := req.Headers()[string(types.HeaderNameX_ICC_CALL_ID)]; !exist {
		return req.CallID()
	} else {
		return traceId // 假设只有一个值
	}
}

func (l *TraceListener) OnDialogCreated(dialog types.Dialog) {
	ctx, exist := l.manager.GetTraceContextByCallID(dialog.CallID())
	if !exist {
		log.GetLogger().Errorf("trace context not found for dialog with Call-ID: %s", dialog.CallID())
		return
	}
	ctx.CreateNewSpan(dialog.ID(), dialog.CallID(), string(types.MethodInvite), dialog.RemoteURI(), dialog.CreatedAt(), dialog.Metadatas())
}

func (l *TraceListener) OnDialogStateChanged(dialog types.Dialog) {
	// 一般dialog状态变化时不需要更新Segment和Span
}

func (l *TraceListener) OnDialogTerminated(dialog types.Dialog) {
	log.GetLogger().Debugf("Dialog terminated: %s", dialog.ID())
	ctx, exist := l.manager.GetTraceContextByCallID(dialog.CallID())
	if !exist {
		log.GetLogger().Errorf("trace context not found for dialog with Call-ID: %s", dialog.CallID())
		return
	}
	ctx.FinishExistSpan(dialog.ID(), false, dialog.UpdatedAt()) // 我们认为事务有成功与失败，会话没有
	data := sniffdata.WrapWithSniffData(ctx.segment)            // 发送Segment
	l.submit(data)                                              // 提交Segment到输出通道
	// l.manager.RemoveTraceContextByCallID(dialog.CallID()) // TODO会话结束后移除
}

func (l *TraceListener) OnTransactionCreated(transaction types.Transaction) {
	ctx, exist := l.manager.GetTraceContextByCallID(transaction.Request().CallID())
	if !exist {
		log.GetLogger().Errorf("trace context not found for dialog with Call-ID: %s", transaction.Request().CallID())
		return
	}
	// 创建新的Span
	callID := transaction.Request().CallID()
	method := transaction.Request().MethodAsString()
	startTime := transaction.CreatedAt()
	headers := transaction.Request().Headers()
	switch transaction.UA() {
	case types.UAClient:
		remoteURI, _ := utils.ExtractURIAndTag(transaction.Request().To())
		ctx.CreateNewSpan(transaction.ID(), callID, method, remoteURI, startTime, headers)
	case types.UAServer:
		// 对于服务器端请求，使用From作为对端地址
		remoteURI, _ := utils.ExtractURIAndTag(transaction.Request().From())
		ctx.CreateNewSpan(transaction.ID(), callID, method, remoteURI, startTime, headers)
	}
}

func (l *TraceListener) OnTransactionStateChanged(transaction types.Transaction) {
	// 一般dialog状态变化时不需要更新Segment和Span
}

func (l *TraceListener) OnTransactionTerminated(tx types.Transaction) {
	log.GetLogger().Debugf("Transaction terminated: %s", tx.ID())
	ctx, exist := l.manager.GetTraceContextByCallID(tx.Request().CallID())
	if !exist {
		log.GetLogger().Errorf("trace context not found for dialog with Call-ID: %s", tx.Request().CallID())
		return
	}
	isError := tx.LastResponse() != nil && tx.LastResponse().Status() >= 300
	// 结束现有的Span
	ctx.FinishExistSpan(tx.ID(), isError, tx.UpdatedAt())
	switch tx.Request().Method() {
	case types.MethodInvite, types.MethodInfo, types.MethodBye, types.MethodCancel:
		// 对于这些方法，我们不需要发送Segment,由dialog生命周期发送
		return
	default:
		data := sniffdata.WrapWithSniffData(ctx.segment) // 发送Segment
		l.submit(data)                                   // 提交Segment到输出通道
	}
}

func (l *TraceListener) OnTransactionTimeout(tx types.Transaction) {
	ctx, exist := l.manager.GetTraceContextByCallID(tx.Request().CallID())
	if !exist {
		log.GetLogger().Errorf("trace context not found for dialog with Call-ID: %s", tx.Request().CallID())
		return
	}
	ctx.FinishExistSpan(tx.ID(), true, tx.UpdatedAt()) // 超时场景一定是错误
	switch tx.Request().Method() {
	case types.MethodInfo, types.MethodBye, types.MethodCancel:
		// 对于这些方法，我们不需要发送Segment
		return
	default:
		data := sniffdata.WrapWithSniffData(ctx.segment) // 发送Segment
		l.submit(data)                                   // 提交Segment到输出通道
	}
}

func (l *TraceListener) OnTransactionError(tx types.Transaction, err error) {
	ctx, exist := l.manager.GetTraceContextByCallID(tx.Request().CallID())
	if !exist {
		log.GetLogger().Errorf("trace context not found for dialog with Call-ID: %s", tx.Request().CallID())
		return
	}
	ctx.FinishExistSpan(tx.ID(), true, tx.UpdatedAt()) // 会话错误场景一定是错误
	switch tx.Request().Method() {
	case types.MethodInfo, types.MethodBye, types.MethodCancel:
		// 对于这些方法，我们不需要发送Segment
		return
	default:
		data := sniffdata.WrapWithSniffData(ctx.segment) // 发送Segment
		l.submit(data)                                   // 提交Segment到输出通道
	}
}
