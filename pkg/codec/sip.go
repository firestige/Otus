package codec

import (
	p "firestige.xyz/otus/pkg/pipeline"
)

type sipDecoder struct {
	decoder
}

func (d *sipDecoder) supported(data p.PacketData) bool {

}
