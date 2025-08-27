package capture

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"firestige.xyz/otus/internal/log"
	otus "firestige.xyz/otus/internal/otus/api"
	capture "firestige.xyz/otus/internal/otus/module/capture/api"
	"firestige.xyz/otus/internal/otus/module/capture/codec"
	"firestige.xyz/otus/internal/otus/module/capture/handle"
	parser "firestige.xyz/otus/plugins/parser/api"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/pcap"
	"github.com/sirupsen/logrus"
)

type Capture struct {
	config *capture.Config

	partitionCount int
	partitions     []*Partition
	outputChannels []chan<- *otus.OutputPacketContext

	shutdownOnce sync.Once
	running      int32
}

type Partition struct {
	id            int
	fanoutGroupID uint16
	handle        handle.CaptureHandle
	decoder       *codec.Decoder
	outputCh      chan<- *otus.OutputPacketContext
}

// TODO capture的生命周期管理？
func (c *Capture) PostConstruct() error {
	log.GetLogger().WithField("pipe", c.config.PipeName).Info("capture module is preparing...")

	for i := 0; i < c.partitionCount; i++ {
		if c.IsChannelSet(i) {
			c.partitions[i].outputCh = c.outputChannels[i]
		}
	}

	for _, partition := range c.partitions {
		if err := c.buildPartitionComponents(partition, c.config); err != nil {
			return fmt.Errorf("failed to build partition %d components: %w", partition.id, err)
		}

		// 如果 handle 实现了 PostConstruct，也调用它
		if postConstructable, ok := partition.handle.(interface{ PostConstruct() error }); ok {
			if err := postConstructable.PostConstruct(); err != nil {
				return fmt.Errorf("failed to post construct handle for partition %d: %w", partition.id, err)
			}
		}
	}
	log.GetLogger().WithFields(logrus.Fields{
		"pipe":       c.config.PipeName,
		"partitions": c.partitionCount,
	}).Infof("%s capture module is prepared", c.config.HandleConfig.CaptureType)
	return nil
}

// 延迟构建分区组件，在 PostConstruct 阶段调用
func (c *Capture) buildPartitionComponents(partition *Partition, cfg *capture.Config) error {
	// 构建 parsers
	parsers := make([]codec.Parser, 0)
	for _, parserCfg := range cfg.ParserConfig {
		// TODO satellite 如何实现只需要name 就可以加载的？
		parsers = append(parsers, parser.GetParser(parserCfg))
	}

	// 创建 parser composite
	parserComposite := codec.NewParserComposite(parsers...)

	// 获取分区对应的 packetQueue（这时候已经初始化了）
	packetQueue := c.getPartitionPacketQueue(partition.id)

	// 创建 transport handlers
	tcpHandler := codec.NewTCPHandler(packetQueue, parserComposite)
	udpHandler := codec.NewUDPHandler(packetQueue, parserComposite)
	transportHandler := codec.NewTransportHandlerComposite(tcpHandler, udpHandler)

	// 创建 decoder
	decoder := codec.NewDecoder(cfg.CodecConfig)
	decoder.SetTransportHandler(transportHandler)

	// 创建 handle
	handle, err := handle.HandleFactory().CreateHandle(cfg.HandleConfig)
	if err != nil {
		return err
	}

	// 绑定到分区
	partition.handle = handle
	partition.decoder = decoder

	return nil
}

// 获取分区对应的 packet queue
func (c *Capture) getPartitionPacketQueue(partitionID int) chan<- *otus.OutputPacketContext {
	// 这里可以是分区专用的队列，或者共享队列
	if partitionID < len(c.outputChannels) && c.outputChannels[partitionID] != nil {
		return c.outputChannels[partitionID]
	}
	// 或者返回一个默认的临时队列，稍后在 SetOutputChannel 中替换
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
	err := partition.handle.Open()
	if err != nil {
		log.GetLogger().WithFields(logrus.Fields{
			"pipe":      c.config.PipeName,
			"partition": partition.id,
			"error":     err,
		}).Error("error opening handle")
		return
	}
	// packetSource := partition.handle.GetTPacketSource()
	for {
		select {
		case <-ctx.Done():
			return
			// case packet, ok := <-packetSource.Packets():
			// 	if !ok {
			// 		return
			// 	}
			// 	if packet == nil {
			// 		continue
			// 	}
			// 	log.GetLogger().Infof("packet captured: %d bytes", len(packet.Data()))
		default:

			data, ci, err := partition.handle.ReadPacket()
			if err != nil {
				// 忽略超时错误；其它错误记录并继续
				if errors.Is(err, pcap.NextErrorTimeoutExpired) ||
					errors.Is(err, afpacket.ErrTimeout) ||
					strings.Contains(strings.ToLower(err.Error()), "timeout") {
					// 超时，不视为错误
					continue
				}
				log.GetLogger().WithFields(logrus.Fields{
					"pipe":      c.config.PipeName,
					"partition": partition.id,
					"error":     err,
				}).Error("error reading packet")
				continue
			}
			log.GetLogger().Infof("packet captured: %d bytes", len(data))
			err = partition.decoder.Decode(data, &ci)
			if err != nil {
				log.GetLogger().WithFields(logrus.Fields{
					"pipe":      c.config.PipeName,
					"partition": partition.id,
					"error":     err,
				}).Error("error decoding packet")
			}
		}
	}
}

func (c *Capture) Shutdown() {
	log.GetLogger().WithField("pipe", c.config.PipeName).Info("capture module is shutting down...")
	c.shutdownOnce.Do(func() {
		atomic.StoreInt32(&c.running, 0)
		for _, partition := range c.partitions {
			if err := partition.handle.Close(); err != nil {
				log.GetLogger().WithFields(logrus.Fields{
					"pipe":      c.config.PipeName,
					"partition": partition.id,
					"error":     err,
				}).Error("error closing handle")
			}
		}
	})
	log.GetLogger().WithField("pipe", c.config.PipeName).Info("capture module is shutting down complete")
}

func (c *Capture) PartitionCount() int {
	return c.config.Partition
}

func (c *Capture) SetOutputChannel(partition int, ch chan<- *otus.OutputPacketContext) error {
	if partition < 0 || partition >= c.partitionCount {
		return fmt.Errorf("invalid partition index: %d", partition)
	}
	if ch == nil {
		return fmt.Errorf("output channel cannot be nil")
	}
	if c.outputChannels[partition] != nil {
		return fmt.Errorf("output channel for partition %d is already set", partition)
	}
	c.outputChannels[partition] = ch
	log.GetLogger().Infof("set channel for partition %d", partition)
	return nil
}

func (c *Capture) IsChannelSet(partition int) bool {
	return c.outputChannels[partition] != nil
}
