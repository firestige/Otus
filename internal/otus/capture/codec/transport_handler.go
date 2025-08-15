package codec

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

type TransportHandler interface {
	support(packet gopacket.Packet) bool
	handle(packet gopacket.Packet) error
}

type transportHandlerComposite struct {
	handlers []TransportHandler
}

func NewTransportHandlerComposite(handlers ...TransportHandler) TransportHandler {
	return &transportHandlerComposite{
		handlers: handlers,
	}
}

func (t *transportHandlerComposite) support(packet gopacket.Packet) bool {
	for _, handler := range t.handlers {
		if handler.support(packet) {
			return true
		}
	}
	return false
}

func (t *transportHandlerComposite) handle(packet gopacket.Packet) error {
	for _, handler := range t.handlers {
		if handler.support(packet) {
			return handler.handle(packet)
		}
	}
	return nil
}

type udpHandler struct {
}

func (u *udpHandler) support(packet gopacket.Packet) bool {
	//检查packet是不是含有UDP层
	udpLayer := packet.Layer(layers.LayerTypeUDP)
	return udpLayer != nil
}

func (u *udpHandler) handle(packet gopacket.Packet) error {
	// 处理UDP包的逻辑
	// 这里从plugin的parser扩展中找到合适的处理器并将其解析成message再送到对应消息的packetChannel中由组装好的协议pipeline处理
	return nil
}

type tcpHandler struct {
}

func (t *tcpHandler) support(packet gopacket.Packet) bool {
	// 检查packet是不是含有TCP层
	return packet.Layer(layers.LayerTypeTCP) != nil
}

func (t *tcpHandler) handle(packet gopacket.Packet) error {
	// 把tcp包放入assembly中进行处理
	return nil // 处理TCP包的逻辑
}
