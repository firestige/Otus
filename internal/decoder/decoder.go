package decoder

import (
	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/config"
	"firestige.xyz/otus/internal/otus/factory"
	"github.com/google/gopacket"
)

const Name = "default_decoder"

type Decoder struct {
}

type DecoderCfg struct {
	config.DecoderConfig
}

func NewDecoder(cfg *DecoderCfg) (*Decoder, error) {
	return &Decoder{}, nil
}

func init() {
	// 注册 Decoder 组件到工厂
	fn := func(cfg interface{}) interface{} {
		decoderCfg, ok := cfg.(*DecoderCfg)
		if !ok {
			return nil
		}
		decoder, err := NewDecoder(decoderCfg)
		if err != nil {
			return nil
		}
		return decoder
	}

	factory.Register(otus.ComponentTypeDecoder, Name, fn)
}

func (d *Decoder) Decode(data []byte, info gopacket.CaptureInfo) (*otus.NetPacket, error) {
	return nil, nil
}
