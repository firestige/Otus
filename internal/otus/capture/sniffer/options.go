package sniffer

type Options struct {
	NetworkInterface string `mapstructure:"network_interface"`
	BlockSize        int    `mapstructure:"block_size"`
}
