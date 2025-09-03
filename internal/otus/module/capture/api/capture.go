package api

import (
	"firestige.xyz/otus/internal/otus/event"
	module "firestige.xyz/otus/internal/otus/module/api"
)

type Capture interface {
	module.Module

	PartitionCount() int
	// SetOutputChannel 由pipe注入输出通道（生产端写入）
	SetOutputChannel(partition int, ch chan<- *event.EventContext) error
	// IsChannelSet 检查指定分区的通道是否已设置
	IsChannelSet(partition int) bool
}
