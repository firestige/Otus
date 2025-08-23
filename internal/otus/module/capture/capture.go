package capture

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"firestige.xyz/otus/internal/log"
	otus "firestige.xyz/otus/internal/otus/api"
	capture "firestige.xyz/otus/internal/otus/module/capture/api"
	"firestige.xyz/otus/internal/otus/module/capture/handle"
	"github.com/sirupsen/logrus"
)

type Capture struct {
	config *capture.Config

	partitionCount int
	partitions     []*Partition
	outputChannels []chan *otus.BatchePacket

	shutdownOnce sync.Once
	running      int32
}

type Partition struct {
	id            int
	fanoutGroupID uint16
	handle        handle.CaptureHandle
	outputCh      chan *otus.BatchePacket

	batchSize    int
	batchTimeout time.Duration
	currentBatch []*otus.BatchePacket
}

// TODO capture的生命周期管理？
func (c *Capture) PostConstruct() error {
	log.GetLogger().WithField("pipe", c.config.PipeName).Info("capture module is preparing...")

	for i := 0; i < c.partitionCount; i++ {
		if c.outputChannels[i] == nil {
			c.outputChannels[i] = make(chan *otus.BatchePacket, c.partitions[i].batchSize)
		}
		c.partitions[i].outputCh = c.outputChannels[i]
	}

	for i, partition := range c.partitions {
		factory := handle.HandleFactory()
		if !factory.IsTypeSupported(c.config.HandleConfig.CaptureType) {
			return fmt.Errorf("Unsupport Handle")
		}
		handle, err := factory.CreateHandle(context.Background(), c.config.HandleConfig.CaptureType)
		if err != nil {
			return fmt.Errorf("Failed to create handle for partition %d: %w", i, err)
		}
		partition.handle = handle
	}
	log.GetLogger().WithFields(logrus.Fields{
		"pipe":       c.config.PipeName,
		"partitions": c.partitionCount,
	}).Infof("%s capture module is prepared", c.config.HandleConfig.CaptureType)
	return nil
}

func (c *Capture) Boot(ctx context.Context) {
	log.GetLogger().WithField("pipe", c.config.PipeName).Info("capture module is starting...")

	atomic.StoreInt32(&c.running, 1)

	wg := &sync.WaitGroup{}
	wg.Add(c.partitionCount)

	// 启动每个partition 的 go routine
	for i, partition := range c.partitions {
		go c.runPartition(ctx, partition, wg)
		log.GetLogger().WithFields(logrus.Fields{
			"pipe":      c.config.PipeName,
			"partition": i,
		}).Info("partition started")
	}

	wg.Wait()
	log.GetLogger().WithField("pipe", c.config.PipeName).Info("capture module stopped")
}

func (c *Capture) runPartition(ctx context.Context, partition *Partition, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() {
		if partition.outputCh != nil {
			close(partition.outputCh)
		}
		log.GetLogger().WithFields(logrus.Fields{
			"pipe":      c.config.PipeName,
			"partition": partition.id,
		}).Info("partition capture routine closed")
	}()

	//
}

func (c *Capture) Shutdown() {

}
