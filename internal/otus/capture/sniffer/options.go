package sniffer

type Options struct {
	NetworkInterface string `mapstructure:"network_interface"` // 网络接口名称
	BlockSize        int    `mapstructure:"block_size"`        // 块大小
	SnapLen          int    `mapstructure:"snap_len"`          // 捕获长度
	BufferSize       int    `mapstructure:"buffer_size"`       // 缓冲区大小
	SupportVlan      bool   `mapstructure:"support_vlan"`      // 是否支持 VLAN
	Timeout          int    `mapstructure:"timeout"`           // 超时时间 (毫秒)
	Filter           string `mapstructure:"filter"`            // BPF 过滤器
	FanoutId         uint16 `mapstructure:"fanout_id"`         // Fanout ID (可选)
	CaptureType      string `mapstructure:"capture_type"`      // 抓包类型 (afpacket, pcap, xdp)
}
