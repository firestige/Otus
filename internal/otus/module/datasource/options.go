package datasource

type Options struct {
	NetworkInterface string `mapstructure:"network_interface"` // 网络接口名称
	SnapLen          int    `mapstructure:"snap_len"`          // 捕获长度
	BufferSize       int    `mapstructure:"buffer_size"`       // 缓冲区大小
	Timeout          int    `mapstructure:"timeout"`           // 超时时间 (毫秒)
	Filter           string `mapstructure:"filter"`            // BPF 过滤器
	FanoutId         uint16 `mapstructure:"fanout_id"`         // Fanout ID
}
