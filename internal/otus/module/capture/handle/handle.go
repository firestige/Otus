package handle

import "github.com/google/gopacket"

// CaptureType 定义抓包类型
type CaptureType string

const (
	TypeAFPacket CaptureType = "afpacket"
	TypePCAP     CaptureType = "pcap"
	TypeXDP      CaptureType = "xdp"
)

// CaptureHandle 定义抓包句柄接口
type CaptureHandle interface {
	// Open 打开抓包句柄
	Open() error

	// ReadPacket 读取数据包
	ReadPacket() ([]byte, gopacket.CaptureInfo, error)

	// Close 关闭抓包句柄
	Close() error

	// GetType 获取抓包类型
	GetType() CaptureType

	GetTPacketSource() *gopacket.PacketSource
}
