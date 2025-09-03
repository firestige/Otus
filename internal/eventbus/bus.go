package eventbus

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"

	"firestige.xyz/otus/internal/log"
	"github.com/serialx/hashring"
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
	hashRing       *hashring.HashRing // 一致性哈希环
	partitionNodes []string           // 分区节点标识

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
		partitionNodes: make([]string, partitionCount),
	}

	// 初始化分区节点标识
	for i := 0; i < partitionCount; i++ {
		bus.partitionNodes[i] = "partition-" + strconv.Itoa(i)
	}

	// 创建一致性哈希环
	bus.hashRing = hashring.New(bus.partitionNodes)

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

	// 根据 Key 使用一致性哈希计算分区
	partitionID := b.getPartitionID(event.Key)
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

// getPartitionID 使用一致性哈希算法计算分区ID
func (b *InMemoryEventBus) getPartitionID(key string) int {
	// 使用一致性哈希环获取节点
	node, ok := b.hashRing.GetNode(key)
	if !ok {
		// 如果哈希环为空，回退到简单哈希
		return 0
	}

	// 从节点名称中提取分区ID (格式: "partition-N")
	for i, partitionNode := range b.partitionNodes {
		if partitionNode == node {
			return i
		}
	}

	// 理论上不应该到这里，但作为保险回退到0
	return 0
}

// AddPartition 动态添加分区（可选功能）
func (b *InMemoryEventBus) AddPartition() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if atomic.LoadInt32(&b.closed) == 1 {
		return fmt.Errorf("event bus is closed")
	}

	newPartitionID := len(b.partitions)
	newNodeName := "partition-" + strconv.Itoa(newPartitionID)

	// 创建新分区
	ctx, cancel := context.WithCancel(context.Background())
	newPartition := &partition{
		id:      newPartitionID,
		queue:   make(chan *Event, b.queueSize),
		ctx:     ctx,
		cancel:  cancel,
		handler: b.getHandler,
	}

	// 更新数据结构
	b.partitions = append(b.partitions, newPartition)
	b.partitionNodes = append(b.partitionNodes, newNodeName)
	b.partitionCount++

	// 更新一致性哈希环
	b.hashRing = b.hashRing.AddNode(newNodeName)

	// 启动新分区
	go b.runPartition(newPartition)

	log.GetLogger().Infof("Added new partition: %d", newPartitionID)
	return nil
}

// RemovePartition 动态移除分区（可选功能）
func (b *InMemoryEventBus) RemovePartition(partitionID int) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if atomic.LoadInt32(&b.closed) == 1 {
		return fmt.Errorf("event bus is closed")
	}

	if partitionID < 0 || partitionID >= len(b.partitions) {
		return fmt.Errorf("invalid partition ID: %d", partitionID)
	}

	// 不允许移除最后一个分区
	if len(b.partitions) <= 1 {
		return fmt.Errorf("cannot remove the last partition")
	}

	// 关闭指定分区
	partition := b.partitions[partitionID]
	partition.cancel()
	close(partition.queue)

	// 从哈希环中移除节点
	nodeName := b.partitionNodes[partitionID]
	b.hashRing = b.hashRing.RemoveNode(nodeName)

	// 从切片中移除（注意：这会改变后续分区的索引）
	// 在生产环境中，可能需要更复杂的重平衡策略
	b.partitions = append(b.partitions[:partitionID], b.partitions[partitionID+1:]...)
	b.partitionNodes = append(b.partitionNodes[:partitionID], b.partitionNodes[partitionID+1:]...)
	b.partitionCount--

	log.GetLogger().Infof("Removed partition: %d", partitionID)
	return nil
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
