package pipeline

import "time"

type PacketData interface {
}

type CaptureInfo interface {
	GetTimestamp() time.Time
	GetLenth() int
	GetCaptureLength() int
	IsTruncated() bool
}

type DataSource interface {
	ReadPacketData() (PacketData, CaptureInfo, error)
	ZeroCopyReadPacketData() (PacketData, CaptureInfo, error)
}

type Decoder interface {
	supported(data PacketData) bool
	Decode(data PacketData) (interface{}, error)
}

type Filter interface {
	Filter(data interface{}, chain FilterChain) error
}

type FilterChain interface {
	Filter(data interface{}) error
}

type Dispatcher interface {
	Dispatch(data interface{}) error
}

type Sender interface {
	Support(data interface{}) bool
	Send(data interface{}) error
}

type pipeline struct {
	nc *capture.netCapture
}

func (p *pipeline) Start() error {
	return nil
}

func (p *pipeline) Stop() error {
}
