package codec

import (
	"firestige.xyz/otus/internal/log"
	otus "firestige.xyz/otus/internal/otus/api"
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
	output chan<- *otus.OutputPacketContext
	parser Parser
}

func NewUDPHandler(output chan<- *otus.OutputPacketContext, p Parser) TransportHandler {
	return &udpHandler{
		output: output,
		parser: p,
	}
}

func (u *udpHandler) support(packet *IPv4Packet) bool {
	//检查packet是不是含有UDP层
	return packet.Protocol == layers.IPProtocolUDP
}

func (u *udpHandler) handle(packet *IPv4Packet) error {
	payload := packet.Payload

	if !u.parser.Detect(payload) {
		// 快速探测，不能处理的消息直接返回错误
		// TODO: 这里可以考虑添加日志记录，另外，应该根据探测结果返回异常，这里直接返回SIP错误是不够的
		return ErrNotSIP
	}
	// 单个UDP包可能含有多个应用层消息（主要针对其他协议为了提高吞吐量，SIP不存在这种情况）
	for len(payload) > 0 {
		fiveTuple := extractFiveTuple(packet)
		msg, n, err := u.parser.Extract(payload)
		if err != nil {
			return err
		}
		p := &otus.NetPacket{
			Protocol:  layers.IPProtocolUDP,
			Timestamp: packet.Timestamp.UnixNano(),
			FiveTuple: &fiveTuple,
			Payload:   msg,
		}
		ctx := make(map[string]*otus.NetPacket)
		ctx[string(p.ApplicationProtoType)] = p
		outputCtx := &otus.OutputPacketContext{
			Context: ctx,
		}
		log.GetLogger().Infof("UDP parsed packet: %+v", p)
		u.output <- outputCtx
		payload = payload[n:]
	}
	return nil
}

type tcpHandler struct {
	assembly TCPAssembly // TCP assembly负责所有TCP处理逻辑
}

func NewTCPHandler(output chan<- *otus.OutputPacketContext, p Parser) TransportHandler {
	// 创建consumer函数，将重组后的消息封装为OutputPacketContext发送到output
	consumer := func(data []byte, fiveTuple *otus.FiveTuple, timestamp int64) error {
		p := &otus.NetPacket{
			Protocol:  layers.IPProtocolTCP,
			Timestamp: timestamp,
			FiveTuple: fiveTuple,
			Payload:   data,
		}
		ctx := make(map[string]*otus.NetPacket)
		ctx[string(p.ApplicationProtoType)] = p
		outputCtx := &otus.OutputPacketContext{
			Context: ctx,
		}
		select {
		case output <- outputCtx:
			return nil
		default:
			return nil // 如果通道满了，静默丢弃
		}
	}

	// 创建TCP assembly，传入consumer和parser
	assembly := NewTCPAssembly(consumer, p)

	return &tcpHandler{
		assembly: assembly,
	}
}

func (t *tcpHandler) support(packet *IPv4Packet) bool {
	// 检查packet是不是含有TCP层
	return packet.Protocol == layers.IPProtocolTCP
}

func (t *tcpHandler) handle(packet *IPv4Packet) error {
	// 将所有TCP处理工作委派给assembly
	return t.assembly.ProcessPacket(packet)
}
