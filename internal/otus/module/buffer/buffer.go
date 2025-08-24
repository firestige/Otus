package buffer

import (
	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/internal/otus/api"
)

type Offset struct {
	Partition int
	Position  int
}

type BatchBuffer struct {
	buf       []*api.OutputPacketContext
	first     Offset
	last      Offset
	size      int
	cap       int
	partition int
}

func NewBatchBuffer(cap, partition int) *BatchBuffer {
	return &BatchBuffer{
		buf:       make([]*api.OutputPacketContext, cap),
		cap:       cap,
		partition: partition,
	}
}

func (b *BatchBuffer) Buf() []*api.OutputPacketContext {
	return b.buf
}

func (b *BatchBuffer) First() Offset {
	return b.first
}

func (b *BatchBuffer) Last() Offset {
	return b.last
}

func (b *BatchBuffer) Len() int {
	return b.size
}

func (b *BatchBuffer) Add(data *api.OutputPacketContext) {
	if b.size == b.cap {
		log.GetLogger().Errorf("cannot add one item to the fulling BatchBuffer, the capacity is %d", b.cap)
		return // Buffer is full, cannot add more data
	}
	if data == nil {
		log.GetLogger().Error("cannot add nil data to BatchBuffer")
		return
	}
	// 计算当前消息的 offset
	currentOffset := Offset{
		Partition: b.partition,
		Position:  b.size, // 使用当前 size 作为在批次中的位置
	}
	if b.size == 0 {
		b.first = currentOffset
	}
	b.last = currentOffset
	b.buf[b.size] = data
	b.size++
	log.GetLogger().Debugf("added packet to batch buffer, partition=%d, position=%d, total_size=%d",
		currentOffset.Partition, currentOffset.Position, b.size)
}
