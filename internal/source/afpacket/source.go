package afpacket

import (
	"context"
	"os"

	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/config"
	"firestige.xyz/otus/internal/otus/factory"
	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"golang.org/x/net/bpf"
)

const Name = "afpacket"

type AfCfg struct {
	config.SourceConfig `mapstructure:",squash"`
	Device              string `mapstructure:"device"`
	SnapLen             int    `mapstructure:"snap_len"`
	BufferSizeMB        int    `mapstructure:"buffer_size_mb"`
	TimeoutMs           int    `mapstructure:"timeout_ms"`
	FanoutID            uint16 `mapstructure:"fanout_id"`
	BpfFilter           string `mapstructure:"bpf_filter"`
}

type Source struct {
	handle *afpacket.TPacket

	device    string
	frameSize int
	blockSize int
	numBlocks int
	timeoutMs int
	fanoutID  uint16
	bpfFilter string
}

func init() {
	fn := func(cfg interface{}) interface{} {
		afCfg, ok := cfg.(*AfCfg)
		if !ok {
			return nil
		}
		s, err := NewSource(afCfg)
		if err != nil {
			return nil
		}
		return s
	}
	factory.Register(otus.ComponentTypeSource, Name, fn)
}

func NewSource(cfg *AfCfg) (s otus.Source, err error) {
	pageSize := os.Getpagesize()
	frameSize, blockSize, numBlocks, err := recomputeSize(cfg.BufferSizeMB, cfg.SnapLen, pageSize)
	if err != nil {
		return nil, err
	}
	rs := &Source{
		device:    cfg.Device,
		frameSize: frameSize,
		blockSize: blockSize,
		numBlocks: numBlocks,
		timeoutMs: cfg.TimeoutMs,
	}
	err = rs.open()
	return rs, err
}

func (s *Source) open() error {
	tp, err := afpacket.NewTPacket(
		afpacket.OptInterface(s.device),
		afpacket.OptFrameSize(s.frameSize),
		afpacket.OptBlockSize(s.blockSize),
		afpacket.OptNumBlocks(s.numBlocks),
		afpacket.OptPollTimeout(s.timeoutMs),
		afpacket.SocketRaw,
		afpacket.TPacketVersion3,
	)
	if err != nil {
		return err
	}

	if s.fanoutID > 0 {
		err = tp.SetFanout(afpacket.FanoutHashWithDefrag, s.fanoutID)
		if err != nil {
			return err
		}
	}

	if s.bpfFilter != "" {
		pcapBPF, err := pcap.CompileBPFFilter(layers.LinkTypeEthernet, s.frameSize, s.bpfFilter)
		if err != nil {
			return err
		}
		rawBPF := make([]bpf.RawInstruction, len(pcapBPF))
		for i, inst := range pcapBPF {
			rawBPF[i] = bpf.RawInstruction{
				Op: inst.Code,
				Jt: inst.Jt,
				Jf: inst.Jf,
				K:  inst.K,
			}
		}
		err = tp.SetBPF(rawBPF)
		if err != nil {
			return err
		}
	}
	s.handle = tp
	return err
}

func (s *Source) Start(ctx context.Context) error {
	h, err := afpacket.NewTPacket(
		afpacket.OptInterface(s.device),
		afpacket.OptFrameSize(s.frameSize),
		afpacket.OptBlockSize(s.blockSize),
		afpacket.OptNumBlocks(s.numBlocks),
		afpacket.OptPollTimeout(s.timeoutMs),
		afpacket.SocketRaw,
		afpacket.TPacketVersion3,
	)
	if err != nil {
		return err
	}

	s.handle = h
	return nil
}

func (s *Source) ReadPacket() (data []byte, info gopacket.CaptureInfo, err error) {
	return s.handle.ReadPacketData()
}

func (s *Source) Stop() error {
	s.handle.Close()
	return nil
}
