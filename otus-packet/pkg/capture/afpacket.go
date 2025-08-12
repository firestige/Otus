package capture

import (
	"fmt"
	"net"
	"time"

	"firestige.xyz/otus/internal/utils"
	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
)

// afpacketHandle AF_PACKET 抓包句柄实现
type afpacketHandle struct {
	tpacket       *afpacket.TPacket
	interfaceName string
	options       *CaptureOptions
	stats         *HandleStats
}

// NewAFPacketHandle 创建新的 AF_PACKET 抓包句柄
func NewAFPacketHandle() CaptureHandle {
	return &afpacketHandle{
		stats: &HandleStats{},
	}
}

// Open 打开 AF_PACKET 抓包句柄
func (h *afpacketHandle) Open(interfaceName string, options *CaptureOptions) error {
	if options == nil {
		options = DefaultCaptureOptions()
	}

	h.interfaceName = interfaceName
	h.options = options

	// 获取网络接口
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return fmt.Errorf("failed to get interface %s: %v", interfaceName, err)
	}

	// 创建 AF_PACKET socket
	tpacket, err := afpacket.NewTPacket(
		afpacket.OptInterface(iface.Name),
		afpacket.OptFrameSize(options.SnapLen),
		afpacket.OptBlockSize(options.BufferSize),
		afpacket.OptNumBlocks(64),
		afpacket.OptAddVLANHeader(true),
		afpacket.OptPollTimeout(time.Duration(options.Timeout)*time.Millisecond),
	)
	if err != nil {
		return fmt.Errorf("failed to create TPacket: %v", err)
	}

	h.tpacket = tpacket

	// Fanout 支持
	if options.FanoutId > 0 {
		if err := tpacket.SetFanout(afpacket.FanoutHashWithDefrag, options.FanoutId); err != nil {
			return fmt.Errorf("failed to set fanout: %v", err)
		}

	}

	// 如果有 BPF 过滤器，则设置过滤器
	if options.Filter != "" {
		rawBpf, err := utils.CompileBpf(options.Filter)
		if err != nil {
			return fmt.Errorf("failed to compile BPF filter: %v", err)
		}
		tpacket.SetBPF(rawBpf)
	}

	return nil
}

// ReadPacket 读取数据包
func (h *afpacketHandle) ReadPacket() (data []byte, ci gopacket.CaptureInfo, err error) {
	if h.tpacket == nil {
		return nil, ci, fmt.Errorf("handle not opened")
	}

	data, ci, err = h.tpacket.ReadPacketData()
	if err != nil {
		h.stats.Errors++
		return nil, ci, err
	}

	h.stats.PacketsReceived++

	return
}

func (h *afpacketHandle) ZeroCopyReadPacket() (data []byte, ci gopacket.CaptureInfo, err error) {
	if h.tpacket == nil {
		return nil, ci, fmt.Errorf("handle not opened")
	}

	data, ci, err = h.tpacket.ZeroCopyReadPacketData()
	if err != nil {
		h.stats.Errors++
		return nil, ci, err
	}

	h.stats.PacketsReceived++

	return
}

// Close 关闭抓包句柄
func (h *afpacketHandle) Close() error {
	if h.tpacket != nil {
		h.tpacket.Close()
		h.tpacket = nil
	}
	return nil
}

// GetStats 获取抓包统计信息
func (h *afpacketHandle) GetStats() (*HandleStats, error) {
	if h.tpacket == nil {
		return nil, fmt.Errorf("handle not opened")
	}

	// 获取 AF_PACKET 的统计信息
	stats, err := h.tpacket.Stats()
	if err != nil {
		return h.stats, err
	}

	// 更新统计信息 (根据实际的 afpacket.Stats 字段)
	h.stats.PacketsReceived = uint64(stats.Packets)
	// 注意: afpacket.Stats 可能没有 Drops 字段，需要根据实际情况调整
	// h.stats.PacketsDropped = uint64(stats.Drops)

	return h.stats, nil
}

// GetType 获取抓包类型
func (h *afpacketHandle) GetType() CaptureType {
	return TypeAFPacket
}

// SetPromiscuous 设置混杂模式 (AF_PACKET 特有方法)
func (h *afpacketHandle) SetPromiscuous(enable bool) error {
	if h.tpacket == nil {
		return fmt.Errorf("handle not opened")
	}

	// AF_PACKET 混杂模式设置需要额外实现
	// 这里只是占位符
	return fmt.Errorf("promiscuous mode setting not implemented yet")
}

// GetInterfaceName 获取接口名称
func (h *afpacketHandle) GetInterfaceName() string {
	return h.interfaceName
}

// GetOptions 获取配置选项
func (h *afpacketHandle) GetOptions() *CaptureOptions {
	return h.options
}
