package buffer

import (
	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/internal/otus/api"
)

type BatchBuffer struct {
	buf   []*api.OutputPacketContext
	first *api.Offset
	last  *api.Offset
	size  int
	cap   int
}

func NewBatchBuffer(cap int) *BatchBuffer {
	return &BatchBuffer{
		buf: make([]*api.OutputPacketContext, cap),
		cap: cap,
	}
}

func (b *BatchBuffer) Buf() []*api.OutputPacketContext {
	return b.buf
}

func (b *BatchBuffer) First() *api.Offset {
	return b.first
}

func (b *BatchBuffer) Last() *api.Offset {
	return b.last
}

func (b *BatchBuffer) Len() int {
	return b.size
}

func (b *BatchBuffer) Add(data *api.OutputPacketContext) {
	if b.size == b.cap {
		log.GetLogger().Errorf("cannot add one item to the fulling BatchBuffer, the capacity is %d", b.cap)
		return // Buffer is full, cannot add more data
	} else if data.Offset == nil {
		log.GetLogger().Error("cannot add one item to BatchBuffer because the input data is illegal, the offset is empty")
		return
	}
	if b.size == 0 {
		b.first = data.Offset
	}
	b.last = data.Offset
	b.buf[b.size] = data
	b.size++
}
