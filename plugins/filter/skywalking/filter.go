package skywalking

import (
	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/log"
	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/event"
	"firestige.xyz/otus/plugins/filter/skywalking/session"
	"firestige.xyz/otus/plugins/filter/skywalking/types"
)

var (
	Name     = "skywalking_filter"
	ShowName = "Skywalking Filter"
)

type Filter struct {
	config.CommonFields

	ServiceName     string `mapstructure:"service_name"`     // 服务名称
	ServiceInstance string `mapstructure:"service_instance"` // 服务实例
	LocalIp         string `mapstructure:"local_ip"`         // 本地IP地址，接收SIP消息的IP地址

	parser  *SipParser
	handler *session.SessionHandler
}

func (f *Filter) Name() string {
	return Name
}

func (f *Filter) ShowName() string {
	return ShowName
}

func (f *Filter) DefaultConfig() string {
	return ``
}

func (f *Filter) PostConstruct() error {

}

func (f *Filter) Filter(e *event.EventContext) {
	p, err := event.GetValue[otus.NetPacket](e, "netpacket")
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
	f.handler.HandleMessage(sipMsg)
}

func (f *Filter) analyseDirection(five *otus.FiveTuple) types.Direction {
	if five.SrcIP.String() == f.LocalIp {
		return types.DirectionOutbound
	} else if five.DstIP.String() == f.LocalIp {
		return types.DirectionInbound
	}
	log.GetLogger().Warnf("Unknown direction for connection: %s", five)
	return types.DirectionUnknown
}
