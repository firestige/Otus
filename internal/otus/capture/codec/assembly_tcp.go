package codec

import (
	"sync"

	"firestige.xyz/otus/internal/otus/api"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/tcpassembly"
)

// TCPStreamConsumerFunc TCP流数据的消费者函数类型
// 参数:
//   - data: parser提取出的应用层消息数据
//   - fiveTuple: 连接的五元组信息
//   - timestamp: 数据包时间戳（纳秒）
//
// tcpHandler通过传入此函数来决定如何处理重组后的数据
type TCPStreamConsumerFunc func(data []byte, fiveTuple *api.FiveTuple, timestamp int64) error

// TCPAssembly 接口，屏蔽底层gopacket实现细节
// 负责TCP segment的重组、流量重整、消息的有序性和完整性保证
// tcpHandler将所有TCP处理工作委派给TCPAssembly
type TCPAssembly interface {
	// ProcessPacket 处理IPv4Packet，负责：
	// 1. 从IPv4Packet中解析TCP头和五元组信息
	// 2. 进行TCP流重组和排序
	// 3. 调用parser提取应用层消息
	// 4. 通过consumer函数处理提取出的消息
	ProcessPacket(packet *IPv4Packet) error

	// Close 关闭assembly并清理所有连接资源
	Close() error
}

// NewTCPAssembly 创建TCP Assembly实例的工厂函数
// 参数:
//   - consumer: 处理提取出的消息的回调函数
//   - parser: 用于解析应用层消息的parser
//
// 返回: TCPAssembly实例
func NewTCPAssembly(consumer TCPStreamConsumerFunc, parser Parser) TCPAssembly {
	streamFactory := &tcpStreamFactory{
		consumer: consumer,
		parser:   parser,
	}
	streamPool := tcpassembly.NewStreamPool(streamFactory)
	assembler := tcpassembly.NewAssembler(streamPool)

	return &tcpAssemblyImpl{
		assembler:     assembler,
		streamFactory: streamFactory,
	}
}

// tcpAssemblyImpl TCPAssembly的具体实现
type tcpAssemblyImpl struct {
	assembler     *tcpassembly.Assembler
	streamFactory *tcpStreamFactory
}

// ProcessPacket 实现TCPAssembly接口
func (t *tcpAssemblyImpl) ProcessPacket(packet *IPv4Packet) error {
	if packet.Protocol != layers.IPProtocolTCP {
		return nil // 不是TCP包，忽略
	}

	// 从IPv4Packet构建gopacket.Packet
	tcpLayer := &layers.TCP{}

	// 解析TCP层
	decoded := []gopacket.LayerType{}
	packetData := packet.Payload
	parser := gopacket.NewDecodingLayerParser(layers.LayerTypeTCP, tcpLayer)
	if err := parser.DecodeLayers(packetData, &decoded); err != nil {
		return err
	}

	// 构建五元组用于流标识
	fiveTuple := api.FiveTuple{
		SrcIP:    packet.SrcIP,
		DstIP:    packet.DstIP,
		SrcPort:  uint16(tcpLayer.SrcPort),
		DstPort:  uint16(tcpLayer.DstPort),
		Protocol: packet.Protocol,
	}

	// 创建网络流和传输层流
	netFlow, err := gopacket.FlowFromEndpoints(
		layers.NewIPEndpoint(packet.SrcIP),
		layers.NewIPEndpoint(packet.DstIP))
	if err != nil {
		return err
	}

	// 将五元组和时间戳信息传递给stream factory
	t.streamFactory.setContextInfo(&fiveTuple, packet.Timestamp.UnixNano())

	// 将TCP数据包送入assembler进行重组
	t.assembler.AssembleWithTimestamp(netFlow, tcpLayer, packet.Timestamp)

	return nil
}

// Close 实现TCPAssembly接口
func (t *tcpAssemblyImpl) Close() error {
	t.assembler.FlushAll()
	return nil
}

// tcpStreamFactory 实现tcpassembly.StreamFactory接口
type tcpStreamFactory struct {
	consumer TCPStreamConsumerFunc
	parser   Parser
	mutex    sync.RWMutex

	// 上下文信息，用于传递给新创建的stream
	currentFiveTuple *api.FiveTuple
	currentTimestamp int64
}

// setContextInfo 设置当前处理包的上下文信息
func (f *tcpStreamFactory) setContextInfo(fiveTuple *api.FiveTuple, timestamp int64) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.currentFiveTuple = fiveTuple
	f.currentTimestamp = timestamp
}

// New 实现tcpassembly.StreamFactory接口
func (f *tcpStreamFactory) New(net, transport gopacket.Flow) tcpassembly.Stream {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	// 复制当前的五元组和时间戳信息
	var fiveTuple *api.FiveTuple
	var timestamp int64
	if f.currentFiveTuple != nil {
		fiveTupleCopy := *f.currentFiveTuple
		fiveTuple = &fiveTupleCopy
		timestamp = f.currentTimestamp
	}

	return &tcpStream{
		net:       net,
		transport: transport,
		consumer:  f.consumer,
		parser:    f.parser,
		fiveTuple: fiveTuple,
		timestamp: timestamp,
		buffer:    make([]byte, 0),
	}
}

// tcpStream 实现tcpassembly.Stream接口
type tcpStream struct {
	net, transport gopacket.Flow
	consumer       TCPStreamConsumerFunc
	parser         Parser
	fiveTuple      *api.FiveTuple
	timestamp      int64
	buffer         []byte
}

// Reassembled 实现tcpassembly.Stream接口
func (s *tcpStream) Reassembled(reassembled []tcpassembly.Reassembly) {
	for _, r := range reassembled {
		if len(r.Bytes) > 0 {
			s.buffer = append(s.buffer, r.Bytes...)

			// 尝试使用parser提取消息
			if s.parser != nil && s.consumer != nil && s.fiveTuple != nil {
				s.extractMessages()
			}
		}
	}
} // ReassemblyComplete 实现tcpassembly.Stream接口
func (s *tcpStream) ReassemblyComplete() {
	// 流重组完成，处理剩余的数据
	if s.consumer != nil && s.fiveTuple != nil && len(s.buffer) > 0 {
		// 如果还有剩余数据且没有parser，直接发送
		if s.parser == nil {
			s.consumer(s.buffer, s.fiveTuple, s.timestamp)
		} else {
			s.extractMessages()
		}
	}
}

// extractMessages 从缓冲区中提取消息
func (s *tcpStream) extractMessages() {
	for len(s.buffer) > 0 {
		if !s.parser.Detect(s.buffer) {
			break // parser无法识别，等待更多数据
		}

		msg, consumed, err := s.parser.Extract(s.buffer)
		if err != nil {
			break // 解析错误，停止处理
		}

		if consumed == 0 {
			break // 需要更多数据
		}

		// 发送提取出的消息
		if msg != nil {
			s.consumer(msg, s.fiveTuple, s.timestamp)
		}

		// 移除已处理的数据
		s.buffer = s.buffer[consumed:]
	}
}
