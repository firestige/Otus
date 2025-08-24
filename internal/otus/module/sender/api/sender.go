package api

import (
	otus "firestige.xyz/otus/internal/otus/api"
	module "firestige.xyz/otus/internal/otus/module/api"
)

type Sender interface {
	module.Module

	// SetInputChannel 由pipe注入输入通道（消费端读取）
	SetInputChannel(partition int, ch <-chan *otus.OutputPacketContext) error
	// IsChannelSet 检查指定分区的通道是否已设置
	IsChannelSet(partition int) bool
}
