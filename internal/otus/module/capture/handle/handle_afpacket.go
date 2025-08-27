package handle

import (
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/internal/utils"
	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/sirupsen/logrus"
)

// afpacketHandle AF_PACKET 抓包句柄实现
type afpacketHandle struct {
	tpacket *afpacket.TPacket
	options *Options
	ds      gopacket.PacketDataSource
}

// NewAFPacketHandle 创建新的 AF_PACKET 抓包句柄
func NewAFPacketHandle(options *Options) CaptureHandle {
	return &afpacketHandle{
		options: options,
	}
}

// Open 打开 AF_PACKET 抓包句柄
func (h *afpacketHandle) Open() error {
	if h.options == nil {
		h.options = DefaultCaptureOptions()
	}

	// 获取网络接口
	iface, err := net.InterfaceByName(h.options.NetworkInterface)
	if err != nil {
		return fmt.Errorf("failed to get interface %s: %v", h.options.NetworkInterface, err)
	}

	log.GetLogger().WithFields(logrus.Fields{
		"interface": iface.Name,
		"index":     iface.Index,
		"mtu":       iface.MTU,
		"flags":     iface.Flags.String(),
		"hw_addr":   iface.HardwareAddr.String(),
	}).Info("interface details")

	framSize, szBlock, numBlock, err := computeFrameSizeAndBlocks(h.options)
	if err != nil {
		return fmt.Errorf("failed to compute frame size and blocks: %v", err)
	}

	log.GetLogger().WithFields(logrus.Fields{
		"frame_size":  framSize,
		"block_size":  szBlock,
		"num_blocks":  numBlock,
		"buffer_size": h.options.BufferSize,
		"snap_len":    h.options.SnapLen,
	}).Info("tpacket configuration")

	// 创建 AF_PACKET socket
	tpacket, err := afpacket.NewTPacket(
		afpacket.OptInterface(iface.Name),
		afpacket.OptFrameSize(framSize),
		afpacket.OptBlockSize(szBlock),
		afpacket.OptNumBlocks(numBlock),
		afpacket.OptPollTimeout(pcap.BlockForever),
		afpacket.SocketRaw,
		afpacket.TPacketVersion3,
	)
	if err != nil {
		return fmt.Errorf("failed to create TPacket: %v", err)
	}

	h.tpacket = tpacket

	// Fanout 支持 - 添加更详细的诊断
	if h.options.FanoutId > 0 {
		log.GetLogger().Infof("Setting fanout: ID=%d, Type=FanoutHashWithDefrag", h.options.FanoutId)

		if err := tpacket.SetFanout(afpacket.FanoutHashWithDefrag, h.options.FanoutId); err != nil {
			log.GetLogger().Errorf("failed to set fanout: %v", err)
			return fmt.Errorf("failed to set fanout: %v", err)
		}

		log.GetLogger().Infof("Fanout set successfully - Group ID: %d", h.options.FanoutId)

		// 等待一小段时间让 fanout 完全生效
		time.Sleep(100 * time.Millisecond)

		// 检查 fanout 组状态
		h.checkFanoutStatus()
	}

	// 如果有 BPF 过滤器，则设置过滤器
	if h.options.Filter != "" {
		log.GetLogger().WithField("filter", h.options.Filter).Info("compiling and setting BPF filter")

		rawBpf, err := utils.CompileBpf(h.options.Filter, h.options.SnapLen)
		if err != nil {
			return fmt.Errorf("failed to compile BPF filter: %v", err)
		}

		if err := tpacket.SetBPF(rawBpf); err != nil {
			return fmt.Errorf("failed to set BPF filter: %v", err)
		}

		log.GetLogger().Info("BPF filter set successfully")
	}

	h.ds = gopacket.PacketDataSource(h.tpacket)

	log.GetLogger().Info("afpacket handle opened successfully")
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

// 检查 fanout 组状态的辅助方法
func (h *afpacketHandle) checkFanoutStatus() {
	// 使用 ss 命令检查 packet socket 状态
	h.checkWithSS()

	// 检查接口统计信息
	h.checkInterfaceStats()
}

func (h *afpacketHandle) checkWithSS() {
	// 这个方法需要系统有 ss 命令
	// 在 dev container 中应该可用
}

func (h *afpacketHandle) checkInterfaceStats() {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		log.GetLogger().Warnf("failed to read /proc/net/dev: %v", err)
		return
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.Contains(line, h.options.NetworkInterface) {
			log.GetLogger().WithField("interface_stats", strings.TrimSpace(line)).Info("interface statistics")
			break
		}
	}
}

// ReadPacket 读取数据包 - 增强版本，包含详细诊断
func (h *afpacketHandle) ReadPacket() (data []byte, ci gopacket.CaptureInfo, err error) {
	if h.tpacket == nil || h.ds == nil {
		return nil, ci, fmt.Errorf("handle not opened")
	}

	start := time.Now()
	data, ci, err = h.ds.ReadPacketData()
	duration := time.Since(start)

	if err != nil {
		// 检查是否是超时错误
		if err == pcap.NextErrorTimeoutExpired ||
			(err == syscall.EAGAIN) ||
			strings.Contains(strings.ToLower(err.Error()), "timeout") {
			log.GetLogger().WithFields(logrus.Fields{
				"duration": duration,
				"error":    "timeout",
			}).Debug("read packet timeout")
		} else {
			log.GetLogger().WithFields(logrus.Fields{
				"error":    err.Error(),
				"duration": duration,
			}).Error("read packet failed with non-timeout error")
		}
	} else {
		log.GetLogger().WithFields(logrus.Fields{
			"packet_len":     len(data),
			"duration":       duration,
			"timestamp":      ci.Timestamp,
			"capture_length": ci.CaptureLength,
			"length":         ci.Length,
		}).Debug("read packet success")
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

func (h *afpacketHandle) GetTPacketSource() *gopacket.PacketSource {
	return gopacket.NewPacketSource(h.tpacket, layers.LinkTypeEthernet)
}
