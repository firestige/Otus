package codec

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// IPv4FlowKey IPv4分片流标识
type IPv4FlowKey struct {
	Flow gopacket.Flow // 网络流（src/dst IP）
	ID   uint16        // IP包ID
}

// IPv4Fragment IPv4分片信息
type IPv4Fragment struct {
	Data      []byte    // 分片数据（IP载荷）
	Offset    uint16    // 分片偏移量（字节）
	Length    uint16    // 分片数据长度
	IsLast    bool      // 是否为最后一个分片
	Timestamp time.Time // 接收时间
}

// IPv4FragmentBuffer IPv4分片缓冲区
type IPv4FragmentBuffer struct {
	Fragments    []*IPv4Fragment
	TotalSize    uint16       // 预期总大小
	ReceivedSize uint16       // 已接收大小
	HasLastFrag  bool         // 是否收到最后分片
	LastActivity time.Time    // 最后活动时间
	FirstPacket  *layers.IPv4 // 保存第一个分片的IPv4头用于重组
}

// IPv4Reassembler IPv4分片重组器
type IPv4Reassembler struct {
	mu            sync.RWMutex
	fragmentFlows map[IPv4FlowKey]*IPv4FragmentBuffer
	maxAge        time.Duration
	maxFragments  int
	maxIPSize     uint16
}

// ReassemblerOptions 重组器选项
type ReassemblerOptions struct {
	MaxAge       time.Duration // 分片最大存活时间
	MaxFragments int           // 每个流最大分片数
	MaxIPSize    uint16        // IP包最大尺寸
}

// NewIPv4Reassembler 创建IPv4分片重组器
func NewIPv4Reassembler(opts ReassemblerOptions) *IPv4Reassembler {
	if opts.MaxAge == 0 {
		opts.MaxAge = 30 * time.Second
	}
	if opts.MaxFragments == 0 {
		opts.MaxFragments = 100
	}
	if opts.MaxIPSize == 0 {
		opts.MaxIPSize = 65535 // 最大IP包大小
	}

	r := &IPv4Reassembler{
		fragmentFlows: make(map[IPv4FlowKey]*IPv4FragmentBuffer),
		maxAge:        opts.MaxAge,
		maxFragments:  opts.MaxFragments,
		maxIPSize:     opts.MaxIPSize,
	}

	// 启动清理协程
	go r.startCleanupRoutine()
	return r
}

// ProcessIPv4Packet 处理IPv4包，返回重组后的完整IPv4包
func (r *IPv4Reassembler) ProcessIPv4Packet(ip *layers.IPv4, ci *gopacket.CaptureInfo) (*IPv4Packet, error) {
	// 检查是否需要重组
	if r.isCompletePacket(ip) {
		// 完整包，直接返回
		return r.convertToIPv4Packet(ip, ci), nil
	}

	// 验证分片
	if err := r.validateFragment(ip); err != nil {
		return nil, err
	}

	// 处理分片
	return r.handleFragment(ip, ci)
}

// isCompletePacket 判断是否为完整包
func (r *IPv4Reassembler) isCompletePacket(ip *layers.IPv4) bool {
	// 设置了DF标志，或者既没有MF标志也没有偏移
	return (ip.Flags&layers.IPv4DontFragment != 0) ||
		(ip.Flags&layers.IPv4MoreFragments == 0 && ip.FragOffset == 0)
}

// convertToIPv4Packet 将layers.IPv4转换为IPv4Packet
func (r *IPv4Reassembler) convertToIPv4Packet(ip *layers.IPv4, ci *gopacket.CaptureInfo) *IPv4Packet {
	return &IPv4Packet{
		SrcIP:     ip.SrcIP,
		DstIP:     ip.DstIP,
		Protocol:  ip.Protocol,
		ID:        ip.Id,
		Flags:     ip.Flags,
		TTL:       ip.TTL,
		Length:    ip.Length,
		Payload:   ip.Payload,
		Timestamp: ci.Timestamp,
	}
}

// handleFragment 处理分片
func (r *IPv4Reassembler) handleFragment(ip *layers.IPv4, ci *gopacket.CaptureInfo) (*IPv4Packet, error) {
	key := IPv4FlowKey{
		Flow: ip.NetworkFlow(),
		ID:   ip.Id,
	}

	fragment := &IPv4Fragment{
		Data:      make([]byte, len(ip.Payload)),
		Offset:    ip.FragOffset * 8,
		Length:    uint16(len(ip.Payload)),
		IsLast:    ip.Flags&layers.IPv4MoreFragments == 0,
		Timestamp: ci.Timestamp,
	}
	copy(fragment.Data, ip.Payload)

	r.mu.Lock()
	defer r.mu.Unlock()

	// 获取或创建缓冲区
	buffer, exists := r.fragmentFlows[key]
	if !exists {
		buffer = &IPv4FragmentBuffer{
			Fragments:    make([]*IPv4Fragment, 0, r.maxFragments),
			LastActivity: time.Now(),
			FirstPacket:  ip,
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
		ipPacket, err := r.reassembleIPv4Packet(buffer, ci)
		delete(r.fragmentFlows, key) // 清理已重组的流
		return ipPacket, err
	}

	return nil, nil // 需要更多分片
}

// insertFragment 插入分片到缓冲区（按偏移排序）
func (r *IPv4Reassembler) insertFragment(buffer *IPv4FragmentBuffer, fragment *IPv4Fragment) {
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
				append([]*IPv4Fragment{fragment}, buffer.Fragments[i:]...)...)
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
func (r *IPv4Reassembler) canReassemble(buffer *IPv4FragmentBuffer) bool {
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

// reassembleIPv4Packet 重组IPv4包
func (r *IPv4Reassembler) reassembleIPv4Packet(buffer *IPv4FragmentBuffer, ci *gopacket.CaptureInfo) (*IPv4Packet, error) {
	if buffer.TotalSize > r.maxIPSize {
		return nil, fmt.Errorf("IP packet too large: %d bytes", buffer.TotalSize)
	}

	// 重组IP负载
	payload := make([]byte, 0, buffer.TotalSize)
	for _, frag := range buffer.Fragments {
		payload = append(payload, frag.Data...)
	}

	return &IPv4Packet{
		SrcIP:     buffer.FirstPacket.SrcIP,
		DstIP:     buffer.FirstPacket.DstIP,
		Protocol:  buffer.FirstPacket.Protocol,
		ID:        buffer.FirstPacket.Id,
		Flags:     buffer.FirstPacket.Flags &^ layers.IPv4MoreFragments, // 清除MF标志
		TTL:       buffer.FirstPacket.TTL,
		Length:    uint16(len(payload)) + uint16(buffer.FirstPacket.IHL)*4, // 重新计算长度
		Payload:   payload,
		Timestamp: ci.Timestamp,
	}, nil
}

// validateFragment 验证分片
func (r *IPv4Reassembler) validateFragment(ip *layers.IPv4) error {
	fragSize := uint16(len(ip.Payload))
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
func (r *IPv4Reassembler) startCleanupRoutine() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		r.cleanup()
	}
}

// cleanup 清理过期分片
func (r *IPv4Reassembler) cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for key, buffer := range r.fragmentFlows {
		if now.Sub(buffer.LastActivity) > r.maxAge {
			delete(r.fragmentFlows, key)
		}
	}
}

// GetStats 获取统计信息
func (r *IPv4Reassembler) GetStats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return map[string]interface{}{
		"active_flows":  len(r.fragmentFlows),
		"max_age":       r.maxAge,
		"max_fragments": r.maxFragments,
		"max_ip_size":   r.maxIPSize,
	}
}
