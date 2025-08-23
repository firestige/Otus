package handle

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"firestige.xyz/otus/internal/utils"
	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
)

// afpacketHandle AF_PACKET 抓包句柄实现
type afpacketHandle struct {
	ctx     context.Context
	tpacket *afpacket.TPacket
	options *Options
}

// NewAFPacketHandle 创建新的 AF_PACKET 抓包句柄
func NewAFPacketHandle(ctx context.Context) CaptureHandle {
	return &afpacketHandle{
		ctx: ctx,
	}
}

// Open 打开 AF_PACKET 抓包句柄
func (h *afpacketHandle) Open(options *Options) error {
	if options == nil {
		options = DefaultCaptureOptions()
	}
	h.options = options

	// 获取网络接口
	iface, err := net.InterfaceByName(options.NetworkInterface)
	if err != nil {
		return fmt.Errorf("failed to get interface %s: %v", options.NetworkInterface, err)
	}

	framSize, szBlock, numBlock, err := computeFrameSizeAndBlocks(options)
	if err != nil {
		return fmt.Errorf("failed to compute frame size and blocks: %v", err)
	}

	// 创建 AF_PACKET socket
	tpacket, err := afpacket.NewTPacket(
		afpacket.OptInterface(iface.Name),
		afpacket.OptFrameSize(framSize),
		afpacket.OptBlockSize(szBlock),
		afpacket.OptNumBlocks(numBlock),
		afpacket.OptPollTimeout(time.Duration(options.Timeout)*time.Millisecond),
		afpacket.SocketRaw,
		afpacket.TPacketVersion3,
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
		rawBpf, err := utils.CompileBpf(options.Filter, options.SnapLen)
		if err != nil {
			return fmt.Errorf("failed to compile BPF filter: %v", err)
		}
		tpacket.SetBPF(rawBpf)
	}

	return nil
}

func computeFrameSizeAndBlocks(options *Options) (frameSize int, blockSize int, numBlocks int, err error) {
	pageSize := os.Getpagesize()
	if options.SnapLen < pageSize {
		frameSize = pageSize / (pageSize / options.SnapLen)
	} else {
		frameSize = (options.SnapLen/pageSize + 1) * pageSize
	}
	blockSize = frameSize * 128
	numBlocks = options.BufferSize / blockSize

	if numBlocks < 1 {
		return 0, 0, 0, fmt.Errorf("buffer size too small for frame size %d", frameSize)
	}
	return frameSize, blockSize, numBlocks, nil
}

// ReadPacket 读取数据包
func (h *afpacketHandle) ReadPacket() (data []byte, ci gopacket.CaptureInfo, err error) {
	if h.tpacket == nil {
		return nil, ci, fmt.Errorf("handle not opened")
	}

	data, ci, err = h.tpacket.ReadPacketData()
	if err != nil {
		return nil, ci, err
	}

	return
}

func (h *afpacketHandle) ZeroCopyReadPacket() (data []byte, ci gopacket.CaptureInfo, err error) {
	if h.tpacket == nil {
		return nil, ci, fmt.Errorf("handle not opened")
	}

	data, ci, err = h.tpacket.ZeroCopyReadPacketData()
	if err != nil {
		return nil, ci, err
	}

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

// GetType 获取抓包类型
func (h *afpacketHandle) GetType() CaptureType {
	return TypeAFPacket
}

// GetInterfaceName 获取接口名称
func (h *afpacketHandle) GetInterfaceName() string {
	return h.options.NetworkInterface
}

// GetOptions 获取配置选项
func (h *afpacketHandle) GetOptions() *Options {
	return h.options
}
