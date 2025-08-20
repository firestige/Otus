package codec

type Options struct {
	MaxAge          int    `mapstructure:"max_age"` // 最大重组时间（秒）
	MaxFragmentsNum int    `mapstructure:"max_fragments_num"`
	MaxIPSize       uint16 `mapstructure:"max_ip_size"` // 最大IP包大小（字节）
}
