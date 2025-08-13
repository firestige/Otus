package codec

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/tcpassembly"
)

// tcpStreamFactory TCP流工厂
type tcpStreamFactory struct {
	processor *IPv4PacketProcessor
}

func (factory *tcpStreamFactory) New(netFlow, tcpFlow gopacket.Flow) tcpassembly.Stream {
	return &tcpStream{
		netFlow:   netFlow,
		tcpFlow:   tcpFlow,
		processor: factory.processor,
	}
}

// tcpStream TCP流实现
type tcpStream struct {
	netFlow   gopacket.Flow
	tcpFlow   gopacket.Flow
	processor *IPv4PacketProcessor
}

func (stream *tcpStream) Reassembled(reassembly []tcpassembly.Reassembly) {
	// 简化的重组处理
	for _, r := range reassembly {
		if len(r.Bytes) > 0 {
			// 这里可以处理重组后的TCP数据
			// 可以将重组后的数据发送到应用层处理
		}
	}
}

func (stream *tcpStream) ReassemblyComplete() {
	// TCP流重组完成
	// 可以在这里进行清理工作
}
