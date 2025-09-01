package skywalkingtracing

import (
	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/log"
	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/plugins/reporter/skywalkingtracing/session"
	"firestige.xyz/otus/plugins/reporter/skywalkingtracing/types"
)

var (
	Name     = "skywalking_trace_reporter"
	ShowName = "Skywalking Trace Reporter"
)

type Reporter struct {
	config.CommonFields

	ServiceName     string `mapstructure:"service_name"`     // 服务名称
	ServiceInstance string `mapstructure:"service_instance"` // 服务实例
	LocalIp         string `mapstructure:"local_ip"`         // 本地IP地址，接收SIP消息的IP地址

	parser  *SipParser
	handler *session.SessionHandler
}

func (r *Reporter) Name() string {
	return Name
}

func (r *Reporter) ShowName() string {
	return ShowName
}

func (r *Reporter) DefaultConfig() string {
	return ``
}

func (r *Reporter) PostConstruct() error {
	return nil
}

func (r *Reporter) Report(batch otus.BatchPacket) error {
	for _, packet := range batch {
		r.processPacket(packet)
	}
	return nil
}

func (r *Reporter) processPacket(packet *otus.NetPacket) {
	goSipMsg, err := r.parser.Parse(packet.Payload)
	if err != nil {
		return
	}
	direction := r.analyseDirection(packet.FiveTuple)

	conn := &types.Connection{
		SrcIp:     packet.FiveTuple.SrcIP.String(),
		SrcPort:   int(packet.FiveTuple.SrcPort),
		DstIp:     packet.FiveTuple.DstIP.String(),
		DstPort:   int(packet.FiveTuple.DstPort),
		Protocol:  packet.FiveTuple.Protocol.String(),
		Direction: direction,
	}
	sipMsg := FromGoSip(goSipMsg, conn, packet.Timestamp)
	r.handler.HandleMessage(sipMsg)
}

func (r *Reporter) analyseDirection(five *otus.FiveTuple) types.Direction {
	if five.SrcIP.String() == r.LocalIp {
		return types.DirectionOutbound
	} else if five.DstIP.String() == r.LocalIp {
		return types.DirectionInbound
	}
	log.GetLogger().Warnf("Unknown direction for connection: %s", five)
	return types.DirectionUnknown
}

func (r *Reporter) SupportProtocol() string {
	return "SIP"
}

func (r *Reporter) ReportType() {
}
