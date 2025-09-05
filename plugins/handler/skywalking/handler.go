package skywalking

import (
	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/log"
	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/event"
	processor "firestige.xyz/otus/internal/otus/module/processor/api"
	"firestige.xyz/otus/plugins/handler/skywalking/dialog"
	"firestige.xyz/otus/plugins/handler/skywalking/transaction"
	"firestige.xyz/otus/plugins/handler/skywalking/types"
)

var (
	Name     = "skywalking_handler"
	ShowName = "Skywalking Handler"
)

type Handler struct {
	config.CommonFields

	ServiceName     string `mapstructure:"service_name"`     // 服务名称
	ServiceInstance string `mapstructure:"service_instance"` // 服务实例
	LocalIp         string `mapstructure:"local_ip"`         // 本地IP地址，接收SIP消息的IP地址

	parser             *SipParser
	dialogHandler      *dialog.Handler
	transactionHandler *transaction.Handler
}

func (f *Handler) Name() string {
	return Name
}

func (f *Handler) ShowName() string {
	return ShowName
}

func (f *Handler) DefaultConfig() string {
	return ``
}

func (f *Handler) PostConstruct() error {
	return nil
}

func (f *Handler) Handle(ex *processor.Exchange) {
	p, err := event.GetValue[otus.NetPacket](ex.GetEvent(), "netpacket")
	if err != nil {
		return
	}
	goSipMsg, err := f.parser.Parse(p.Payload)
	if err != nil {
		return
	}
	direction := f.analyseDirection(p.FiveTuple)

	conn := &types.Connection{
		SrcIp:     p.FiveTuple.SrcIP.String(),
		SrcPort:   int(p.FiveTuple.SrcPort),
		DstIp:     p.FiveTuple.DstIP.String(),
		DstPort:   int(p.FiveTuple.DstPort),
		Protocol:  p.FiveTuple.Protocol.String(),
		Direction: direction,
	}
	sipMsg := FromGoSip(goSipMsg, conn, p.Timestamp)
	log.GetLogger().
		WithField("Call-id", sipMsg.CallID()).
		WithField("CSeq", sipMsg.CSeq()).
		Debugf("Handling SIP message: %s", sipMsg.StartLine())

	// dialog必须分开处理的原因是terminated事件的处理顺序影响了上报
	// 2. Dialog 处理请求
	if req, ok := sipMsg.(types.SipRequest); ok {
		err := f.dialogHandler.Handle(req)
		if err != nil {
			log.GetLogger().Warnf("Error handling request in dialog: %v", err)
		}
	}

	// 3. Transaction 处理
	err = f.transactionHandler.Handle(sipMsg)
	if err != nil {
		log.GetLogger().Errorf("Error handling message in transaction: %v", err)
	}

	// 4. Dialog 处理响应
	if resp, ok := sipMsg.(types.SipResponse); ok {
		err = f.dialogHandler.Handle(resp)
		if err != nil {
			log.GetLogger().Warnf("Error handling request in dialog: %v", err)
		}
	}
	log.GetLogger().WithField("Call-id", sipMsg.CallID()).WithField("CSeq", sipMsg.CSeq()).Debug("Finished handling SIP message")
}

func (f *Handler) analyseDirection(five *otus.FiveTuple) types.Direction {
	if five.SrcIP.String() == f.LocalIp {
		return types.DirectionOutbound
	} else if five.DstIP.String() == f.LocalIp {
		return types.DirectionInbound
	}
	log.GetLogger().Warnf("Unknown direction for connection: %s", five)
	return types.DirectionUnknown
}
