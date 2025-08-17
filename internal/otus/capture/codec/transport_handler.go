package codec

import (
	"firestige.xyz/otus/internal/otus/api"
	"github.com/google/gopacket/layers"
)

type TransportHandler interface {
	support(packet *IPv4Packet) bool
	handle(packet *IPv4Packet) error
}

type transportHandlerComposite struct {
	handlers []TransportHandler
}

func NewTransportHandlerComposite(handlers ...TransportHandler) TransportHandler {
	return &transportHandlerComposite{
		handlers: handlers,
	}
}

func (t *transportHandlerComposite) support(packet *IPv4Packet) bool {
	for _, handler := range t.handlers {
		if handler.support(packet) {
			return true
		}
	}
	return false
}

func (t *transportHandlerComposite) handle(packet *IPv4Packet) error {
	for _, handler := range t.handlers {
		if handler.support(packet) {
			return handler.handle(packet)
		}
	}
	return nil
}

type udpHandler struct {
	output chan *api.NetPacket
}

func (u *udpHandler) support(packet *IPv4Packet) bool {
	//检查packet是不是含有UDP层
	return packet.Protocol == layers.IPProtocolUDP
}

func (u *udpHandler) handle(packet *IPv4Packet) error {
	// 处理UDP包的逻辑
	// 这里从plugin的parser扩展中找到合适的处理器并将其解析成message再送到对应消息的packetChannel中由组装好的协议pipeline处理
	udpMessage := &api.NetPacket{
		Protocol:  layers.IPProtocolUDP,
		Timestamp: packet.Timestamp,
		Flow:      packet.Flow,
		Payload:   packet.Payload,
	}
	u.output <- udpMessage
	return nil
}

type tcpHandler struct {
	// 这里可以添加TCP处理器特有的字段
	output chan *api.NetPacket
}

func (t *tcpHandler) support(packet *IPv4Packet) bool {
	// 检查packet是不是含有TCP层
	return packet.Protocol == layers.IPProtocolTCP
}

func (t *tcpHandler) handle(packet *IPv4Packet) error {
	// 把tcp包放入assembly中进行处理
	return nil // 处理TCP包的逻辑
}
