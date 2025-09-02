package eventbus

import (
	"context"
	"fmt"
	"hash/fnv"
	"sync"
	"sync/atomic"

	"firestige.xyz/otus/internal/log"
)

// EventBus 事件总线接口
type EventBus interface {
	Publish(event *Event) error
	Subscribe(topic string, handler Handler) error
	Close() error
	GetStats() *Stats
}

// Stats 统计信息
type Stats struct {
	PublishedCount int64
	ProcessedCount int64
	PartitionCount int
	QueuedCount    []int
}

// InMemoryEventBus 基于内存的事件总线实现
type InMemoryEventBus struct {
	partitions     []*partition
	partitionCount int
	queueSize      int
	subscribers    map[string]Handler
	mu             sync.RWMutex
	closed         int32

	// 统计信息
	publishedCount int64
	processedCount int64
}

// NewInMemoryEventBus 创建新的内存事件总线
func NewInMemoryEventBus(partitionCount, queueSize int) EventBus {
	bus := &InMemoryEventBus{
		partitionCount: partitionCount,
		queueSize:      queueSize,
		subscribers:    make(map[string]Handler),
		partitions:     make([]*partition, partitionCount),
	}

	// 初始化分区
	for i := 0; i < partitionCount; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		bus.partitions[i] = &partition{
			id:     i,
			queue:  make(chan *Event, queueSize),
			ctx:    ctx,
			cancel: cancel,
		}
		go bus.runPartition(bus.partitions[i])
	}

	return bus
}

// Publish 发布事件
func (b *InMemoryEventBus) Publish(event *Event) error {
	if atomic.LoadInt32(&b.closed) == 1 {
		return fmt.Errorf("event bus is closed")
	}

	// 根据 CallID 计算分区
	partitionID := b.getPartitionID(event.CallID)
	partition := b.partitions[partitionID]

	select {
	case partition.queue <- event:
		atomic.AddInt64(&b.publishedCount, 1)
		return nil
	default:
		return fmt.Errorf("partition %d queue is full", partitionID)
	}
}

// Subscribe 订阅主题
func (b *InMemoryEventBus) Subscribe(topic string, handler Handler) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if atomic.LoadInt32(&b.closed) == 1 {
		return fmt.Errorf("event bus is closed")
	}

	b.subscribers[topic] = handler

	// 更新所有分区的处理器
	for _, partition := range b.partitions {
		partition.handler = b.getHandler
	}

	log.GetLogger().Infof("Subscribed to topic: %s", topic)
	return nil
}

// Close 关闭事件总线
func (b *InMemoryEventBus) Close() error {
	if !atomic.CompareAndSwapInt32(&b.closed, 0, 1) {
		return nil
	}

	// 关闭所有分区
	for _, partition := range b.partitions {
		partition.cancel()
		close(partition.queue)
	}

	log.GetLogger().Info("Event bus closed")
	return nil
}

// GetStats 获取统计信息
func (b *InMemoryEventBus) GetStats() *Stats {
	stats := &Stats{
		PublishedCount: atomic.LoadInt64(&b.publishedCount),
		ProcessedCount: atomic.LoadInt64(&b.processedCount),
		PartitionCount: b.partitionCount,
		QueuedCount:    make([]int, b.partitionCount),
	}

	for i, partition := range b.partitions {
		stats.QueuedCount[i] = len(partition.queue)
	}

	return stats
}

// getPartitionID 根据 CallID 计算分区ID
func (b *InMemoryEventBus) getPartitionID(callID string) int {
	hasher := fnv.New32a()
	hasher.Write([]byte(callID))
	return int(hasher.Sum32()) % b.partitionCount
}

// getHandler 获取主题对应的处理器
func (b *InMemoryEventBus) getHandler(event *Event) error {
	b.mu.RLock()
	handler, exists := b.subscribers[event.Topic]
	b.mu.RUnlock()

	if !exists {
		log.GetLogger().Debugf("No handler for topic: %s", event.Topic)
		return nil
	}

	return handler(event)
}

// runPartition 运行分区消费者
func (b *InMemoryEventBus) runPartition(p *partition) {
	logger := log.GetLogger()
	logger.Infof("Partition %d started", p.id)

	defer func() {
		logger.Infof("Partition %d stopped", p.id)
	}()

	for {
		select {
		case <-p.ctx.Done():
			return

		case event, ok := <-p.queue:
			if !ok {
				return
			}

			if p.handler != nil {
				if err := p.handler(event); err != nil {
					logger.Errorf("Failed to handle event in partition %d: %v", p.id, err)
				} else {
					atomic.AddInt64(&b.processedCount, 1)
				}
			}
		}
	}
}
