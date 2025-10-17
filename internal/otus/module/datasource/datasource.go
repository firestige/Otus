package datasource

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/internal/utils"
	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/sirupsen/logrus"
)

type Source struct {
	tpacket *afpacket.TPacket
	options *Options
	ds      gopacket.PacketDataSource
}

func NewSource(options *Options) *Source {
	return &Source{
		options: options,
	}
}

func (s *Source) Boot() {
	iface, err := net.InterfaceByName("eth0")
	if err != nil {
		log.GetLogger().WithError(err).Fatal("failed to get interface eth0")
	}

	framSize, szBlock, numBlock, err := computeFrameSizeAndBlocks(s.options)
	if err != nil {
		log.GetLogger().WithError(err).Fatal("failed to compute frame size and blocks")
	}

	log.GetLogger().WithFields(logrus.Fields{
		"frame_size":  framSize,
		"block_size":  szBlock,
		"num_blocks":  numBlock,
		"buffer_size": s.options.BufferSize,
		"snap_len":    s.options.SnapLen,
	}).Info("tpacket configuration")

	// 创建 AF_PACKET socket
	tpacket, err := afpacket.NewTPacket(
		afpacket.OptInterface(iface.Name),
		afpacket.OptFrameSize(framSize),
		afpacket.OptBlockSize(szBlock),
		afpacket.OptNumBlocks(numBlock),
		afpacket.OptPollTimeout(100*time.Millisecond),
		afpacket.SocketRaw,
		afpacket.TPacketVersion3,
	)
	if err != nil {
		log.GetLogger().WithError(err).Fatal("failed to create TPacket")
	}

	s.tpacket = tpacket

	// Fanout 支持 - 添加更详细的诊断
	if s.options.FanoutId > 0 {
		log.GetLogger().Infof("Setting fanout: ID=%d, Type=FanoutHashWithDefrag", s.options.FanoutId)

		if err := tpacket.SetFanout(afpacket.FanoutHashWithDefrag, s.options.FanoutId); err != nil {
			log.GetLogger().Errorf("failed to set fanout: %v", err)
		}

		log.GetLogger().Infof("Fanout set successfully - Group ID: %d", s.options.FanoutId)

		// 等待一小段时间让 fanout 完全生效
		time.Sleep(100 * time.Millisecond)

		// 检查 fanout 组状态
		s.checkFanoutStatus()
	}

	// 如果有 BPF 过滤器，则设置过滤器
	if s.options.Filter != "" {
		log.GetLogger().WithField("filter", s.options.Filter).Info("compiling and setting BPF filter")

		rawBpf, err := utils.CompileBpf(s.options.Filter, s.options.SnapLen)
		if err != nil {
			log.GetLogger().WithError(err).Fatal("failed to compile BPF filter")
		}

		if err := tpacket.SetBPF(rawBpf); err != nil {
			log.GetLogger().WithError(err).Fatal("failed to set BPF filter")
		}

		log.GetLogger().Info("BPF filter set successfully")
	}

	s.ds = gopacket.PacketDataSource(s.tpacket)

	log.GetLogger().Info("afpacket handle opened successfully")

}

func (s *Source) Stop() {
	if s.tpacket != nil {
		s.tpacket.Close()
		log.GetLogger().Info("afpacket handle closed")
	}
}

func (s *Source) ReadPacket() (data []byte, ci gopacket.CaptureInfo, err error) {
	if s.tpacket == nil || s.ds == nil {
		return nil, ci, fmt.Errorf("source not prepared")
	}

	start := time.Now()
	data, ci, err = s.ds.ReadPacketData()
	duration := time.Since(start)

	if err != nil {
		if err == afpacket.ErrTimeout || strings.Contains(strings.ToLower(err.Error()), "timeout") {
			log.GetLogger().WithFields(logrus.Fields{
				"duration": duration,
				"error":    "timeout",
			}).Debug("read packet timeout")
		} else {
			log.GetLogger().WithFields(logrus.Fields{
				"packet_len":     len(data),
				"duration":       duration,
				"timestamp":      ci.Timestamp,
				"capture_length": ci.CaptureLength,
				"length":         ci.Length,
			}).Debug("read packet success")
		}
	}
	return
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
func (s *Source) checkFanoutStatus() {
	// 使用 ss 命令检查 packet socket 状态
	s.checkWithSS()

	// 检查接口统计信息
	s.checkInterfaceStats()
}

func (s *Source) checkWithSS() {
	// 这个方法需要系统有 ss 命令
	// 在 dev container 中应该可用
}

func (s *Source) checkInterfaceStats() {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		log.GetLogger().Warnf("failed to read /proc/net/dev: %v", err)
		return
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.Contains(line, s.options.NetworkInterface) {
			log.GetLogger().WithField("interface_stats", strings.TrimSpace(line)).Info("interface statistics")
			break
		}
	}
}
