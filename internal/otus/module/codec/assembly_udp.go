package codec

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// UDPFlowKey UDP流标识
type UDPFlowKey struct {
	Flow gopacket.Flow // 网络流（src/dst IP）
	ID   uint16        // IP包ID
}

// UDPFragment UDP分片信息
type UDPFragment struct {
	IPv4Header *layers.IPv4
	Offset     uint16
	Length     uint16
	IsLast     bool
	Timestamp  time.Time
}

// UDPFragmentBuffer UDP分片缓冲区
type UDPFragmentBuffer struct {
	Fragments    []*UDPFragment
	TotalSize    uint16
	ReceivedSize uint16
	HasLastFrag  bool
	LastActivity time.Time
}

// UDPReassembler UDP重组器
type UDPReassembler struct {
	mu            sync.RWMutex
	fragmentFlows map[UDPFlowKey]*UDPFragmentBuffer
	maxAge        time.Duration
	maxFragments  int
	maxUDPSize    uint16
}

// ReassemblerOptions 重组器选项
type ReassemblerOptions struct {
	MaxAge       time.Duration // 分片最大存活时间
	MaxFragments int           // 每个流最大分片数
	MaxUDPSize   uint16        // UDP大包最大尺寸
}

// NewUDPReassembler 创建UDP重组器
func NewUDPReassembler(opts ReassemblerOptions) *UDPReassembler {
	if opts.MaxAge == 0 {
		opts.MaxAge = 30 * time.Second
	}
	if opts.MaxFragments == 0 {
		opts.MaxFragments = 100
	}
	if opts.MaxUDPSize == 0 {
		opts.MaxUDPSize = 65507 // 65535 - 20(IP) - 8(UDP)
	}

	r := &UDPReassembler{
		fragmentFlows: make(map[UDPFlowKey]*UDPFragmentBuffer),
		maxAge:        opts.MaxAge,
		maxFragments:  opts.MaxFragments,
		maxUDPSize:    opts.MaxUDPSize,
	}

	// 启动清理协程
	go r.startCleanupRoutine()
	return r
}

// ProcessIPv4Packet 处理IPv4包，返回重组后的UDP大包
func (r *UDPReassembler) ProcessIPv4Packet(ip *layers.IPv4) (*UDPPacket, error) {
	// 检查是否为UDP协议
	if ip.Protocol != layers.IPProtocolUDP {
		return nil, errors.New("not a UDP packet")
	}

	// 检查是否需要重组
	if r.isCompletePacket(ip) {
		// 完整包，直接解析UDP
		return r.parseCompleteUDPPacket(ip)
	}

	// 验证分片
	if err := r.validateFragment(ip); err != nil {
		return nil, err
	}

	// 处理分片
	return r.handleFragment(ip)
}

// isCompletePacket 判断是否为完整包
func (r *UDPReassembler) isCompletePacket(ip *layers.IPv4) bool {
	// 设置了DF标志，或者既没有MF标志也没有偏移
	return (ip.Flags&layers.IPv4DontFragment != 0) ||
		(ip.Flags&layers.IPv4MoreFragments == 0 && ip.FragOffset == 0)
}

// parseCompleteUDPPacket 解析完整UDP包
func (r *UDPReassembler) parseCompleteUDPPacket(ip *layers.IPv4) (*UDPPacket, error) {
	if len(ip.Payload) < 8 {
		return nil, errors.New("UDP header too short")
	}

	udpHeader := parseUDPHeader(ip.Payload[:8])
	udpPayload := ip.Payload[8:]

	return &UDPPacket{
		SrcIP:     ip.SrcIP,
		DstIP:     ip.DstIP,
		SrcPort:   udpHeader.SrcPort,
		DstPort:   udpHeader.DstPort,
		Length:    udpHeader.Length,
		Checksum:  udpHeader.Checksum,
		Payload:   udpPayload,
		Timestamp: time.Now(),
	}, nil
}

// handleFragment 处理分片
func (r *UDPReassembler) handleFragment(ip *layers.IPv4) (*UDPPacket, error) {
	key := UDPFlowKey{
		Flow: ip.NetworkFlow(),
		ID:   ip.Id,
	}

	fragment := &UDPFragment{
		IPv4Header: ip,
		Offset:     ip.FragOffset * 8,
		Length:     ip.Length - uint16(ip.IHL)*4,
		IsLast:     ip.Flags&layers.IPv4MoreFragments == 0,
		Timestamp:  time.Now(),
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 获取或创建缓冲区
	buffer, exists := r.fragmentFlows[key]
	if !exists {
		buffer = &UDPFragmentBuffer{
			Fragments:    make([]*UDPFragment, 0, r.maxFragments),
			LastActivity: time.Now(),
		}
		r.fragmentFlows[key] = buffer
	}

	// 检查分片数量限制
	if len(buffer.Fragments) >= r.maxFragments {
		delete(r.fragmentFlows, key)
		return nil, fmt.Errorf("too many fragments for flow")
	}

	// 插入分片
	r.insertFragment(buffer, fragment)

	// 检查是否可以重组
	if r.canReassemble(buffer) {
		udpPacket, err := r.reassembleUDPPacket(buffer)
		delete(r.fragmentFlows, key) // 清理已重组的流
		return udpPacket, err
	}

	return nil, nil // 需要更多分片
}

// insertFragment 插入分片到缓冲区（按偏移排序）
func (r *UDPReassembler) insertFragment(buffer *UDPFragmentBuffer, fragment *UDPFragment) {
	buffer.LastActivity = time.Now()
	buffer.ReceivedSize += fragment.Length

	if fragment.IsLast {
		buffer.HasLastFrag = true
		buffer.TotalSize = fragment.Offset + fragment.Length
	}

	// 按偏移插入分片
	inserted := false
	for i, existing := range buffer.Fragments {
		if fragment.Offset < existing.Offset {
			// 在这个位置插入
			buffer.Fragments = append(buffer.Fragments[:i],
				append([]*UDPFragment{fragment}, buffer.Fragments[i:]...)...)
			inserted = true
			break
		} else if fragment.Offset == existing.Offset {
			// 重复分片，忽略
			return
		}
	}

	if !inserted {
		buffer.Fragments = append(buffer.Fragments, fragment)
	}
}

// canReassemble 检查是否可以重组
func (r *UDPReassembler) canReassemble(buffer *UDPFragmentBuffer) bool {
	if !buffer.HasLastFrag || len(buffer.Fragments) == 0 {
		return false
	}

	// 检查是否有空洞
	expectedOffset := uint16(0)
	for _, frag := range buffer.Fragments {
		if frag.Offset != expectedOffset {
			return false // 有空洞
		}
		expectedOffset = frag.Offset + frag.Length
	}

	return expectedOffset == buffer.TotalSize
}

// reassembleUDPPacket 重组UDP大包
func (r *UDPReassembler) reassembleUDPPacket(buffer *UDPFragmentBuffer) (*UDPPacket, error) {
	if buffer.TotalSize > r.maxUDPSize {
		return nil, fmt.Errorf("UDP packet too large: %d bytes", buffer.TotalSize)
	}

	// 重组IP负载
	payload := make([]byte, 0, buffer.TotalSize)
	firstFragment := buffer.Fragments[0]

	for _, frag := range buffer.Fragments {
		payload = append(payload, frag.IPv4Header.Payload...)
	}

	// 解析UDP头（在第一个分片中）
	if len(payload) < 8 {
		return nil, errors.New("reassembled packet too short for UDP header")
	}

	udpHeader := parseUDPHeader(payload[:8])
	udpPayload := payload[8:]

	return &UDPPacket{
		SrcIP:     firstFragment.IPv4Header.SrcIP,
		DstIP:     firstFragment.IPv4Header.DstIP,
		SrcPort:   udpHeader.SrcPort,
		DstPort:   udpHeader.DstPort,
		Length:    udpHeader.Length,
		Checksum:  udpHeader.Checksum,
		Payload:   udpPayload,
		Timestamp: time.Now(),
	}, nil
}

// validateFragment 验证分片
func (r *UDPReassembler) validateFragment(ip *layers.IPv4) error {
	fragSize := ip.Length - uint16(ip.IHL)*4
	if fragSize == 0 {
		return errors.New("fragment size is zero")
	}

	fragOffset := uint32(ip.FragOffset) * 8
	if fragOffset+uint32(fragSize) > 65535 {
		return errors.New("fragment would exceed maximum IP packet size")
	}

	return nil
}

// startCleanupRoutine 启动清理协程
func (r *UDPReassembler) startCleanupRoutine() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		r.cleanup()
	}
}

// cleanup 清理过期分片
func (r *UDPReassembler) cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for key, buffer := range r.fragmentFlows {
		if now.Sub(buffer.LastActivity) > r.maxAge {
			delete(r.fragmentFlows, key)
		}
	}
}

// UDPPacket 重组后的UDP包
type UDPPacket struct {
	SrcIP     []byte
	DstIP     []byte
	SrcPort   uint16
	DstPort   uint16
	Length    uint16
	Checksum  uint16
	Payload   []byte
	Timestamp time.Time
}

// UDPHeader UDP头结构
type UDPHeader struct {
	SrcPort  uint16
	DstPort  uint16
	Length   uint16
	Checksum uint16
}

// parseUDPHeader 解析UDP头
func parseUDPHeader(data []byte) UDPHeader {
	return UDPHeader{
		SrcPort:  uint16(data[0])<<8 | uint16(data[1]),
		DstPort:  uint16(data[2])<<8 | uint16(data[3]),
		Length:   uint16(data[4])<<8 | uint16(data[5]),
		Checksum: uint16(data[6])<<8 | uint16(data[7]),
	}
}

// GetStats 获取统计信息
func (r *UDPReassembler) GetStats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return map[string]interface{}{
		"active_flows":  len(r.fragmentFlows),
		"max_age":       r.maxAge,
		"max_fragments": r.maxFragments,
		"max_udp_size":  r.maxUDPSize,
	}
}
