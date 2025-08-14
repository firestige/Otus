package codec

import (
	"net"
	"strconv"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/tcpassembly"
)

// tcpStreamFactory TCP流工厂
type tcpStreamFactory struct {
	processor *IPv4PacketProcessor
}

func (factory *tcpStreamFactory) New(netFlow, tcpFlow gopacket.Flow) tcpassembly.Stream {
	// 解析网络流信息
	srcIP, dstIP := netFlow.Endpoints()
	srcPort, dstPort := tcpFlow.Endpoints()

	srcPortNum, _ := strconv.Atoi(srcPort.String())
	dstPortNum, _ := strconv.Atoi(dstPort.String())

	return &tcpStream{
		netFlow:   netFlow,
		tcpFlow:   tcpFlow,
		processor: factory.processor,
		srcIP:     net.ParseIP(srcIP.String()),
		dstIP:     net.ParseIP(dstIP.String()),
		srcPort:   uint16(srcPortNum),
		dstPort:   uint16(dstPortNum),
		startTime: time.Now(),
	}
}

// tcpStream TCP流实现
type tcpStream struct {
	netFlow   gopacket.Flow
	tcpFlow   gopacket.Flow
	processor *IPv4PacketProcessor

	// 流信息
	srcIP     net.IP
	dstIP     net.IP
	srcPort   uint16
	dstPort   uint16
	startTime time.Time
}

func (stream *tcpStream) Reassembled(reassembly []tcpassembly.Reassembly) {
	// 处理重组后的TCP数据
	for _, r := range reassembly {
		if len(r.Bytes) > 0 {
			// 创建重组后的网络消息
			now := time.Now()
			msg := &NetworkMessage{
				IPVersion:       4,
				TransportProto:  6, // TCP
				SourceAddr:      stream.srcIP,
				DestinationAddr: stream.dstIP,
				SourcePort:      stream.srcPort,
				DestinationPort: stream.dstPort,
				TimestampSec:    uint32(now.Unix()),
				TimestampMicro:  uint32(now.Nanosecond() / 1000),
				Content:         r.Bytes,
				TCPFlags:        0, // 重组后的数据不保留具体的TCP标志
			}

			// 将重组后的数据发送到应用层处理
			if stream.processor != nil && stream.processor.applicationHandler != nil {
				processedMsg, err := stream.processor.applicationHandler.ProcessMessage(msg)
				if err == nil && processedMsg != nil {
					// 尝试发送到输出通道
					select {
					case stream.processor.outputChannel <- processedMsg:
						// 成功发送
					default:
						// 通道满了，忽略这个消息
					}
				}
			}
		}
	}
}

func (stream *tcpStream) ReassemblyComplete() {
	// TCP流重组完成
	// 这里可以添加流结束时的清理逻辑
	// 例如记录流的统计信息等
}
