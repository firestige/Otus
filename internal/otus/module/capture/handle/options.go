package handle

import (
	"fmt"
	"strings"
)

type Options struct {
	NetworkInterface string      `mapstructure:"network_interface"` // 网络接口名称
	SnapLen          int         `mapstructure:"snap_len"`          // 捕获长度
	BufferSize       int         `mapstructure:"buffer_size"`       // 缓冲区大小
	Timeout          int         `mapstructure:"timeout"`           // 超时时间 (毫秒)
	Filter           string      `mapstructure:"filter"`            // BPF 过滤器
	FanoutId         uint16      `mapstructure:"fanout_id"`         // Fanout ID (可选)
	CaptureType      CaptureType `mapstructure:"capture_type"`      // 抓包类型 (afpacket, pcap, xdp)
}

// ParseCaptureType 将字符串转换为 CaptureType（不区分大小写、去除空白）
func ParseCaptureType(s string) (CaptureType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "afpacket", "af_packet", "af-packet":
		return TypeAFPacket, nil
	case "pcap":
		return TypePCAP, nil
	case "xdp":
		return TypeXDP, nil
	default:
		return "", fmt.Errorf("unknown capture type: %q", s)
	}
}

// UnmarshalText 实现 encoding.TextUnmarshaler，支持 mapstructure / yaml / json 文本反序列化
func (c *CaptureType) UnmarshalText(text []byte) error {
	t, err := ParseCaptureType(string(text))
	if err != nil {
		return err
	}
	*c = t
	return nil
}
