package codec

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/tcpassembly"
)

// IPv4PacketProcessor IPv4包处理器实现
type IPv4PacketProcessor struct {
	udpReassembler     *UDPReassembler
	tcpAssembler       *tcpassembly.Assembler
	applicationHandler *ApplicationProcessor
	outputChannel      chan<- *NetworkMessage
	metrics            *ProcessorMetrics
	config             *ProcessorConfig

	// 解析器缓存
	layerParser   *gopacket.DecodingLayerParser
	decodedLayers []gopacket.LayerType

	// 协议层缓存
	ethernetLayer layers.Ethernet
	ipv4Layer     layers.IPv4
	tcpLayer      layers.TCP
	udpLayer      layers.UDP
	sctpLayer     layers.SCTP

	// 控制和同步
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running int32
}

// NewIPv4PacketProcessor 创建新的IPv4包处理器
func NewIPv4PacketProcessor(config *ProcessorConfig, outputChan chan<- *NetworkMessage) (*IPv4PacketProcessor, error) {
	if config == nil {
		config = DefaultProcessorConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	processor := &IPv4PacketProcessor{
		config:        config,
		outputChannel: outputChan,
		metrics: &ProcessorMetrics{
			StartTime: time.Now(),
		},
		ctx:    ctx,
		cancel: cancel,
	}

	// 初始化应用层处理器
	processor.applicationHandler = NewApplicationProcessor(processor.metrics)

	// 初始化UDP重组器
	processor.udpReassembler = NewUDPReassembler(ReassemblerOptions{
		MaxAge:       config.FragmentTimeout,
		MaxFragments: 100,
		MaxUDPSize:   65507,
	})

	// 初始化gopacket解析器
	processor.layerParser = gopacket.NewDecodingLayerParser(
		layers.LayerTypeEthernet,
		&processor.ethernetLayer,
		&processor.ipv4Layer,
		&processor.tcpLayer,
		&processor.udpLayer,
		&processor.sctpLayer,
	)
	processor.layerParser.IgnoreUnsupported = true
	processor.decodedLayers = make([]gopacket.LayerType, 0, 10)

	// 初始化TCP重组器
	if config.EnableTCPReassembly {
		streamFactory := &tcpStreamFactory{processor: processor}
		streamPool := tcpassembly.NewStreamPool(streamFactory)
		processor.tcpAssembler = tcpassembly.NewAssembler(streamPool)
	}

	return processor, nil
}

// ProcessPacket 实现PacketProcessor接口
func (p *IPv4PacketProcessor) ProcessPacket(ctx context.Context, rawData []byte, meta *CaptureMetadata) error {
	if atomic.LoadInt32(&p.running) == 0 {
		return fmt.Errorf("processor not started")
	}

	// 解析数据包
	err := p.layerParser.DecodeLayers(rawData, &p.decodedLayers)
	if err != nil {
		atomic.AddUint64(&p.metrics.ProcessingErrors, 1)
		return err
	}

	// 处理IPv4层
	for _, layerType := range p.decodedLayers {
		if layerType == layers.LayerTypeIPv4 {
			return p.handleIPv4Packet(ctx, meta)
		}
	}

	return nil
}

// Start 启动处理器
func (p *IPv4PacketProcessor) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&p.running, 0, 1) {
		return fmt.Errorf("processor already running")
	}

	// 启动后台任务
	p.wg.Add(1)
	go p.maintenanceLoop()

	if p.tcpAssembler != nil {
		p.wg.Add(1)
		go p.tcpAssemblerLoop()
	}

	return nil
}

// Stop 停止处理器
func (p *IPv4PacketProcessor) Stop() error {
	if !atomic.CompareAndSwapInt32(&p.running, 1, 0) {
		return fmt.Errorf("processor not running")
	}

	p.cancel()
	p.wg.Wait()
	return nil
}

// GetMetrics 获取性能指标
func (p *IPv4PacketProcessor) GetMetrics() *ProcessorMetrics {
	return &ProcessorMetrics{
		IPv4Packets:       atomic.LoadUint64(&p.metrics.IPv4Packets),
		TCPPackets:        atomic.LoadUint64(&p.metrics.TCPPackets),
		UDPPackets:        atomic.LoadUint64(&p.metrics.UDPPackets),
		SCTPPackets:       atomic.LoadUint64(&p.metrics.SCTPPackets),
		FragmentedPackets: atomic.LoadUint64(&p.metrics.FragmentedPackets),
		SIPMessages:       atomic.LoadUint64(&p.metrics.SIPMessages),
		RTPPackets:        atomic.LoadUint64(&p.metrics.RTPPackets),
		RTCPPackets:       atomic.LoadUint64(&p.metrics.RTCPPackets),
		ProcessingErrors:  atomic.LoadUint64(&p.metrics.ProcessingErrors),
		StartTime:         p.metrics.StartTime,
	}
}

// Process 处理网络包数据 - 维持原有接口不变
// data: 原始包数据
// ci: 捕获信息(包含时间戳等)
func (p *IPv4PacketProcessor) Process(data []byte, ci *gopacket.CaptureInfo) {
	meta := &CaptureMetadata{
		Timestamp:     ci.Timestamp,
		CaptureLength: ci.CaptureLength,
		PacketLength:  ci.Length,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_ = p.ProcessPacket(ctx, data, meta)
}

// handleIPv4Packet 处理IPv4包
func (p *IPv4PacketProcessor) handleIPv4Packet(ctx context.Context, meta *CaptureMetadata) error {
	atomic.AddUint64(&p.metrics.IPv4Packets, 1)

	// 检查分片
	if p.ipv4Layer.Flags&layers.IPv4MoreFragments != 0 || p.ipv4Layer.FragOffset != 0 {
		atomic.AddUint64(&p.metrics.FragmentedPackets, 1)
	}

	// 使用UDPReassembler处理UDP分片
	if p.ipv4Layer.Protocol == layers.IPProtocolUDP {
		udpPacket, err := p.udpReassembler.ProcessIPv4Packet(&p.ipv4Layer)
		if err == nil && udpPacket != nil {
			msg := &NetworkMessage{
				IPVersion:       4,
				TransportProto:  uint8(layers.IPProtocolUDP),
				SourceAddr:      udpPacket.SrcIP,
				DestinationAddr: udpPacket.DstIP,
				SourcePort:      udpPacket.SrcPort,
				DestinationPort: udpPacket.DstPort,
				TimestampSec:    uint32(meta.Timestamp.Unix()),
				TimestampMicro:  uint32(meta.Timestamp.Nanosecond() / 1000),
				Content:         udpPacket.Payload,
			}
			return p.processTransportMessage(msg)
		}
	}

	// 处理其他协议
	return p.processTransportLayers(meta)
}

// processTransportLayers 处理传输层
func (p *IPv4PacketProcessor) processTransportLayers(meta *CaptureMetadata) error {
	for _, layerType := range p.decodedLayers {
		switch layerType {
		case layers.LayerTypeUDP:
			return p.processUDPLayer(meta)
		case layers.LayerTypeTCP:
			return p.processTCPLayer(meta)
		case layers.LayerTypeSCTP:
			return p.processSCTPLayer(meta)
		}
	}
	return nil
}

// processUDPLayer 处理UDP层
func (p *IPv4PacketProcessor) processUDPLayer(meta *CaptureMetadata) error {
	atomic.AddUint64(&p.metrics.UDPPackets, 1)

	msg := &NetworkMessage{
		IPVersion:       4,
		TransportProto:  uint8(layers.IPProtocolUDP),
		SourceAddr:      p.ipv4Layer.SrcIP,
		DestinationAddr: p.ipv4Layer.DstIP,
		SourcePort:      uint16(p.udpLayer.SrcPort),
		DestinationPort: uint16(p.udpLayer.DstPort),
		TimestampSec:    uint32(meta.Timestamp.Unix()),
		TimestampMicro:  uint32(meta.Timestamp.Nanosecond() / 1000),
		Content:         p.udpLayer.Payload,
	}

	return p.processTransportMessage(msg)
}

// processTCPLayer 处理TCP层
func (p *IPv4PacketProcessor) processTCPLayer(meta *CaptureMetadata) error {
	atomic.AddUint64(&p.metrics.TCPPackets, 1)

	// 计算TCP标志位
	var flags uint8
	if p.tcpLayer.FIN {
		flags |= 0x01
	}
	if p.tcpLayer.SYN {
		flags |= 0x02
	}
	if p.tcpLayer.RST {
		flags |= 0x04
	}
	if p.tcpLayer.PSH {
		flags |= 0x08
	}
	if p.tcpLayer.ACK {
		flags |= 0x10
	}
	if p.tcpLayer.URG {
		flags |= 0x20
	}

	msg := &NetworkMessage{
		IPVersion:       4,
		TransportProto:  uint8(layers.IPProtocolTCP),
		SourceAddr:      p.ipv4Layer.SrcIP,
		DestinationAddr: p.ipv4Layer.DstIP,
		SourcePort:      uint16(p.tcpLayer.SrcPort),
		DestinationPort: uint16(p.tcpLayer.DstPort),
		TimestampSec:    uint32(meta.Timestamp.Unix()),
		TimestampMicro:  uint32(meta.Timestamp.Nanosecond() / 1000),
		Content:         p.tcpLayer.Payload,
		TCPFlags:        flags,
	}

	// 使用TCP流重组器
	if p.config.EnableTCPReassembly && p.tcpAssembler != nil {
		p.tcpAssembler.AssembleWithTimestamp(
			p.ipv4Layer.NetworkFlow(),
			&p.tcpLayer,
			meta.Timestamp,
		)
		return nil
	}

	// 简单TCP处理
	if len(msg.Content) > 0 {
		return p.processTransportMessage(msg)
	}
	return nil
}

// processSCTPLayer 处理SCTP层
func (p *IPv4PacketProcessor) processSCTPLayer(meta *CaptureMetadata) error {
	atomic.AddUint64(&p.metrics.SCTPPackets, 1)

	var content []byte
	if len(p.sctpLayer.Payload) >= 16 {
		switch p.sctpLayer.Payload[8] {
		case 0: // DATA chunk
			content = p.sctpLayer.Payload[16:]
		case 64: // IDATA chunk
			content = p.sctpLayer.Payload[20:]
		default:
			content = p.sctpLayer.Payload[8:]
		}
	}

	msg := &NetworkMessage{
		IPVersion:       4,
		TransportProto:  uint8(layers.IPProtocolSCTP),
		SourceAddr:      p.ipv4Layer.SrcIP,
		DestinationAddr: p.ipv4Layer.DstIP,
		SourcePort:      uint16(p.sctpLayer.SrcPort),
		DestinationPort: uint16(p.sctpLayer.DstPort),
		TimestampSec:    uint32(meta.Timestamp.Unix()),
		TimestampMicro:  uint32(meta.Timestamp.Nanosecond() / 1000),
		Content:         content,
	}

	return p.processTransportMessage(msg)
}

// processTransportMessage 处理传输层消息
func (p *IPv4PacketProcessor) processTransportMessage(msg *NetworkMessage) error {
	processedMsg, err := p.applicationHandler.ProcessMessage(msg)
	if err != nil {
		return err
	}

	if processedMsg != nil {
		select {
		case p.outputChannel <- processedMsg:
			return nil
		default:
			return fmt.Errorf("output channel full")
		}
	}

	return nil
}

// maintenanceLoop 维护循环
func (p *IPv4PacketProcessor) maintenanceLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.MetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			// 这里可以添加定期维护任务
		}
	}
}

// tcpAssemblerLoop TCP重组器循环
func (p *IPv4PacketProcessor) tcpAssemblerLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.tcpAssembler.FlushOlderThan(time.Now().Add(-time.Minute))
		}
	}
}
