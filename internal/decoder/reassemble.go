package decoder

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/gopacket/layers"
)

// IPv4Fragment 表示一个 IPv4 分片
type IPv4Fragment struct {
	data      []byte
	offset    uint16
	moreFrags bool
	timestamp time.Time
}

// IPv4ReassemblyKey 用于标识属于同一个 IP 数据报的分片
type IPv4ReassemblyKey struct {
	srcIP    string
	dstIP    string
	id       uint16
	protocol layers.IPProtocol
}

// IPv4ReassemblyBuffer 存储待重组的分片
type IPv4ReassemblyBuffer struct {
	fragments  []*IPv4Fragment
	totalSize  uint16
	received   map[uint16]bool // 已接收的偏移量
	firstSeen  time.Time
	lastUpdate time.Time
}

// ipv4Reassembler 管理 IPv4 分片重组
type ipv4Reassembler struct {
	buffers map[IPv4ReassemblyKey]*IPv4ReassemblyBuffer
	mu      sync.RWMutex
	timeout time.Duration // 分片超时时间
}

func newIPv4Reassembler(timeout time.Duration) *ipv4Reassembler {
	return &ipv4Reassembler{
		buffers: make(map[IPv4ReassemblyKey]*IPv4ReassemblyBuffer),
		timeout: timeout,
	}
}

// reassembleIPv4 重组 IPv4 分片（调用前已确认是分片包）
func (d *Decoder) reassembleIPv4(ip4 layers.IPv4, timestamp time.Time) (*layers.IPv4, error) {
	// 构建重组键
	key := IPv4ReassemblyKey{
		srcIP:    ip4.SrcIP.String(),
		dstIP:    ip4.DstIP.String(),
		id:       ip4.Id,
		protocol: ip4.Protocol,
	}

	d.reassembler.mu.Lock()
	defer d.reassembler.mu.Unlock()

	// 清理超时的分片缓冲区
	d.cleanupExpiredBuffers(timestamp)

	// 获取或创建重组缓冲区
	buffer, exists := d.reassembler.buffers[key]
	if !exists {
		buffer = &IPv4ReassemblyBuffer{
			fragments:  make([]*IPv4Fragment, 0),
			received:   make(map[uint16]bool),
			firstSeen:  timestamp,
			lastUpdate: timestamp,
		}
		d.reassembler.buffers[key] = buffer
	}

	// 检查是否超时
	if timestamp.Sub(buffer.firstSeen) > d.reassembler.timeout {
		delete(d.reassembler.buffers, key)
		return nil, fmt.Errorf("fragment reassembly timeout")
	}

	// 添加新分片
	fragOffset := ip4.FragOffset * 8 // 偏移量以 8 字节为单位

	// 检查是否已经收到此偏移的分片（防止重复）
	if buffer.received[fragOffset] {
		return nil, fmt.Errorf("duplicate fragment at offset %d", fragOffset)
	}

	fragment := &IPv4Fragment{
		data:      ip4.Payload,
		offset:    fragOffset,
		moreFrags: ip4.Flags&layers.IPv4MoreFragments != 0,
		timestamp: timestamp,
	}

	buffer.fragments = append(buffer.fragments, fragment)
	buffer.received[fragOffset] = true
	buffer.lastUpdate = timestamp

	// 如果是最后一个分片，记录总大小
	if !fragment.moreFrags {
		buffer.totalSize = fragOffset + uint16(len(fragment.data))
	}

	// 检查是否收集完所有分片
	if buffer.totalSize > 0 && d.isReassemblyComplete(buffer) {
		// 重组完成
		reassembled, err := d.assembleFragments(buffer, &ip4)
		if err != nil {
			delete(d.reassembler.buffers, key)
			return nil, err
		}

		// 清理缓冲区
		delete(d.reassembler.buffers, key)
		return reassembled, nil
	}

	// 还需要更多分片
	return nil, fmt.Errorf("waiting for more fragments")
}

// isReassemblyComplete 检查是否收集到了所有分片
func (d *Decoder) isReassemblyComplete(buffer *IPv4ReassemblyBuffer) bool {
	if buffer.totalSize == 0 {
		return false
	}

	// 检查是否所有偏移量都已收到
	var offset uint16
	for offset < buffer.totalSize {
		if !buffer.received[offset] {
			return false
		}
		// 找到下一个偏移量
		found := false
		for _, frag := range buffer.fragments {
			if frag.offset > offset {
				if !found || frag.offset < offset {
					offset = frag.offset
					found = true
				}
			}
		}
		if !found {
			// 没有更大的偏移量了，检查是否覆盖到 totalSize
			break
		}
	}

	return true
}

// assembleFragments 组装分片为完整的 IPv4 包
func (d *Decoder) assembleFragments(buffer *IPv4ReassemblyBuffer, template *layers.IPv4) (*layers.IPv4, error) {
	// 创建完整的载荷缓冲区
	payload := make([]byte, buffer.totalSize)

	// 按偏移量排序并拷贝数据
	for _, frag := range buffer.fragments {
		if frag.offset+uint16(len(frag.data)) > buffer.totalSize {
			return nil, fmt.Errorf("fragment overflow: offset=%d, len=%d, total=%d",
				frag.offset, len(frag.data), buffer.totalSize)
		}
		copy(payload[frag.offset:], frag.data)
	}

	// 创建重组后的 IPv4 包
	reassembled := &layers.IPv4{
		Version:    template.Version,
		IHL:        template.IHL,
		TOS:        template.TOS,
		Length:     uint16(20 + len(payload)), // IP 头部(20) + 载荷
		Id:         template.Id,
		Flags:      0, // 清除分片标志
		FragOffset: 0,
		TTL:        template.TTL,
		Protocol:   template.Protocol,
		Checksum:   0, // 需要重新计算
		SrcIP:      template.SrcIP,
		DstIP:      template.DstIP,
		Options:    template.Options,
		Padding:    template.Padding,
	}
	reassembled.Payload = payload

	return reassembled, nil
}

// cleanupExpiredBuffers 清理超时的分片缓冲区
func (d *Decoder) cleanupExpiredBuffers(now time.Time) {
	expiredKeys := make([]IPv4ReassemblyKey, 0)

	for key, buffer := range d.reassembler.buffers {
		if now.Sub(buffer.firstSeen) > d.reassembler.timeout {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		delete(d.reassembler.buffers, key)
	}
}

// IPv6Fragment 表示一个 IPv6 分片
type IPv6Fragment struct {
	data      []byte
	offset    uint16
	moreFrags bool
	timestamp time.Time
}

// IPv6ReassemblyKey 用于标识属于同一个 IPv6 数据报的分片
type IPv6ReassemblyKey struct {
	srcIP          string
	dstIP          string
	identification uint32 // IPv6 使用 32 位 ID
}

// IPv6ReassemblyBuffer 存储待重组的 IPv6 分片
type IPv6ReassemblyBuffer struct {
	fragments  []*IPv6Fragment
	totalSize  uint16
	received   map[uint16]bool
	nextHeader uint8 // 分片扩展头后的下一个协议
	firstSeen  time.Time
	lastUpdate time.Time
}

// ipv6Reassembler 管理 IPv6 分片重组
type ipv6Reassembler struct {
	buffers map[IPv6ReassemblyKey]*IPv6ReassemblyBuffer
	mu      sync.RWMutex
	timeout time.Duration
}

func newIPv6Reassembler(timeout time.Duration) *ipv6Reassembler {
	return &ipv6Reassembler{
		buffers: make(map[IPv6ReassemblyKey]*IPv6ReassemblyBuffer),
		timeout: timeout,
	}
}

// reassembleIPv6 重组 IPv6 分片（调用前已确认有分片扩展头）
func (d *Decoder) reassembleIPv6(ip6 layers.IPv6, frag layers.IPv6Fragment, timestamp time.Time) (*layers.IPv6, error) {
	// 构建重组键
	key := IPv6ReassemblyKey{
		srcIP:          ip6.SrcIP.String(),
		dstIP:          ip6.DstIP.String(),
		identification: frag.Identification,
	}

	d.ipv6Reassembler.mu.Lock()
	defer d.ipv6Reassembler.mu.Unlock()

	// 清理超时的分片缓冲区
	d.cleanupExpiredIPv6Buffers(timestamp)

	// 获取或创建重组缓冲区
	buffer, exists := d.ipv6Reassembler.buffers[key]
	if !exists {
		buffer = &IPv6ReassemblyBuffer{
			fragments:  make([]*IPv6Fragment, 0),
			received:   make(map[uint16]bool),
			nextHeader: uint8(frag.NextHeader),
			firstSeen:  timestamp,
			lastUpdate: timestamp,
		}
		d.ipv6Reassembler.buffers[key] = buffer
	}

	// 检查是否超时
	if timestamp.Sub(buffer.firstSeen) > d.ipv6Reassembler.timeout {
		delete(d.ipv6Reassembler.buffers, key)
		return nil, nil // 超时，返回 nil
	}

	// 提取分片偏移量和 MF 标志
	// FragmentOffset: 13 bits, MoreFragments: 1 bit (bit 0)
	fragOffset := frag.FragmentOffset * 8 // 以 8 字节为单位
	moreFrags := (frag.FragmentOffset & 0x0001) != 0

	// 检查是否已经收到此偏移的分片（防止重复）
	if buffer.received[fragOffset] {
		return nil, nil // 重复分片，返回 nil
	}

	fragment := &IPv6Fragment{
		data:      frag.Payload,
		offset:    fragOffset,
		moreFrags: moreFrags,
		timestamp: timestamp,
	}

	buffer.fragments = append(buffer.fragments, fragment)
	buffer.received[fragOffset] = true
	buffer.lastUpdate = timestamp

	// 如果是最后一个分片，记录总大小
	if !fragment.moreFrags {
		buffer.totalSize = fragOffset + uint16(len(fragment.data))
	}

	// 检查是否收集完所有分片
	if buffer.totalSize > 0 && d.isIPv6ReassemblyComplete(buffer) {
		// 重组完成
		reassembled, err := d.assembleIPv6Fragments(buffer, &ip6)
		if err != nil {
			delete(d.ipv6Reassembler.buffers, key)
			return nil, nil // 组装失败，返回 nil
		}

		// 清理缓冲区
		delete(d.ipv6Reassembler.buffers, key)
		return reassembled, nil
	}

	// 还需要更多分片，返回 nil
	return nil, nil
}

// isIPv6ReassemblyComplete 检查是否收集到了所有 IPv6 分片
func (d *Decoder) isIPv6ReassemblyComplete(buffer *IPv6ReassemblyBuffer) bool {
	if buffer.totalSize == 0 {
		return false
	}

	// 检查是否所有偏移量都已收到
	var offset uint16
	for offset < buffer.totalSize {
		if !buffer.received[offset] {
			return false
		}
		// 找到下一个偏移量
		found := false
		for _, frag := range buffer.fragments {
			if frag.offset > offset {
				if !found || frag.offset < offset {
					offset = frag.offset
					found = true
				}
			}
		}
		if !found {
			break
		}
	}

	return true
}

// assembleIPv6Fragments 组装分片为完整的 IPv6 包
func (d *Decoder) assembleIPv6Fragments(buffer *IPv6ReassemblyBuffer, template *layers.IPv6) (*layers.IPv6, error) {
	// 创建完整的载荷缓冲区
	payload := make([]byte, buffer.totalSize)

	// 按偏移量排序并拷贝数据
	for _, frag := range buffer.fragments {
		if frag.offset+uint16(len(frag.data)) > buffer.totalSize {
			return nil, fmt.Errorf("IPv6 fragment overflow: offset=%d, len=%d, total=%d",
				frag.offset, len(frag.data), buffer.totalSize)
		}
		copy(payload[frag.offset:], frag.data)
	}

	// 创建重组后的 IPv6 包
	reassembled := &layers.IPv6{
		Version:      template.Version,
		TrafficClass: template.TrafficClass,
		FlowLabel:    template.FlowLabel,
		Length:       uint16(len(payload)), // IPv6 Length 不包含基本头部
		NextHeader:   layers.IPProtocol(buffer.nextHeader),
		HopLimit:     template.HopLimit,
		SrcIP:        template.SrcIP,
		DstIP:        template.DstIP,
	}
	reassembled.Payload = payload

	return reassembled, nil
}

// cleanupExpiredIPv6Buffers 清理超时的 IPv6 分片缓冲区
func (d *Decoder) cleanupExpiredIPv6Buffers(now time.Time) {
	expiredKeys := make([]IPv6ReassemblyKey, 0)

	for key, buffer := range d.ipv6Reassembler.buffers {
		if now.Sub(buffer.firstSeen) > d.ipv6Reassembler.timeout {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		delete(d.ipv6Reassembler.buffers, key)
	}
}
