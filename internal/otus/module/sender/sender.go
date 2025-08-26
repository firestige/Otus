package sender

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"firestige.xyz/otus/internal/log"
	otus "firestige.xyz/otus/internal/otus/api"
	module "firestige.xyz/otus/internal/otus/module/api"
	"firestige.xyz/otus/internal/otus/module/buffer"
	sender "firestige.xyz/otus/internal/otus/module/sender/api"
	fallbacker "firestige.xyz/otus/plugins/fallbacker/api"
	reporter "firestige.xyz/otus/plugins/reporter/api"
	"github.com/sirupsen/logrus"
)

var defaultSenderFlushTime = 1000

type Sender struct {
	config *sender.Config

	reporters  []reporter.Reporter
	fallbacker fallbacker.Fallbacker

	inputs       []<-chan *otus.OutputPacketContext
	flushChannel []chan *buffer.BatchBuffer
	buffers      []*buffer.BatchBuffer
	blocking     int32
	shutdownOnce *sync.Once
}

func (s *Sender) PostConstruct() error {
	log.GetLogger().WithField("pipe", s.config.PipeName).Info("sender module is preparing...")

	for _, reporter := range s.reporters {
		err := reporter.PostConstruct()
		if err != nil {
			return err
		}
	}

	for partition := 0; partition < s.config.Partition; partition++ {
		s.inputs[partition] = make(chan *otus.OutputPacketContext)
		s.buffers[partition] = buffer.NewBatchBuffer(s.config.MaxBufferSize, partition)
		s.flushChannel[partition] = make(chan *buffer.BatchBuffer)
	}

	return nil
}

func (s *Sender) Boot(ctx context.Context) {
	log.GetLogger().WithField("pipe", s.config.PipeName).Info("sender module is starting...")
	wg := &sync.WaitGroup{}
	wg.Add(2*s.config.Partition + 1)
	for partition := 0; partition < s.config.Partition; partition++ {
		go s.store(ctx, partition, wg)
		go s.flush(ctx, partition, wg)
	}
	wg.Wait()
}

func (s *Sender) store(ctx context.Context, partition int, wg *sync.WaitGroup) {
	// TODO 补完待发流程，
	// 1.当发送速度跟不上上游事件生产速度时，需要暂存
	// 2.当开始关闭流程时需要暂存到 flush 队列
	// 不要试图合并两个 defer，此外由于 LIFO 特性，wg.Done 实际上是后执行的那个
	defer wg.Done()
	defer log.GetLogger().WithField("pipe", s.config.PipeName).Info("store routine closed")

	childCtx, _ := context.WithCancel(ctx)
	flushTime := s.config.FlushInterval
	if flushTime <= 0 {
		flushTime = defaultSenderFlushTime
	}
	timeTicker := time.NewTicker(time.Duration(flushTime) * time.Millisecond)
	for {
		if atomic.LoadInt32(&s.blocking) == 1 {
			time.Sleep(100 * time.Millisecond)
			log.GetLogger().WithField("pipe", s.config.PipeName).Warn("client disconnected, blocking...")
			continue
		}
		select {
		case <-childCtx.Done():
			return
		case <-timeTicker.C:
			if s.buffers[partition].Len() >= s.config.MinFlushEvents {
				s.flushChannel[partition] <- s.buffers[partition]
				s.buffers[partition] = buffer.NewBatchBuffer(s.config.MaxBufferSize, partition)
			}
		case e := <-s.inputs[partition]:
			if e == nil {
				continue
			}
			s.buffers[partition].Add(e)
			if s.buffers[partition].Len() == s.config.MaxBufferSize {
				s.flushChannel[partition] <- s.buffers[partition]
				s.buffers[partition] = buffer.NewBatchBuffer(s.config.MaxBufferSize, partition)
			}
		}
	}
}

func (s *Sender) flush(ctx context.Context, partition int, wg *sync.WaitGroup) {
	defer wg.Done()
	defer log.GetLogger().WithField("pipe", s.config.PipeName).Info("flush routine closed")
	childCtx, _ := context.WithCancel(ctx)
	for {
		select {
		case <-childCtx.Done():
			s.Shutdown()
			return
		case b := <-s.flushChannel[partition]:
			// TODO flushChannel 需要补完
			s.consume(b)
		}
	}
}

func (s *Sender) Shutdown() {
	s.shutdownOnce.Do(func() {
		s.shutdown0()
	})
}

func (s *Sender) shutdown0() {
	log.GetLogger().WithField("pipe", s.config.PipeName).Info("sender module is shutting down...")
	wg := &sync.WaitGroup{}
	finished := make(chan struct{}, 1)
	wg.Add(len(s.flushChannel))
	for partition := range s.buffers {
		go func(p int) {
			defer wg.Done()
			s.consume(s.buffers[p])
		}(partition)
	}
	go func() {
		wg.Wait()
		close(finished)
	}()

	ticker := time.NewTicker(module.ShutdownHookTime)
	select {
	case <-ticker.C:
		for _, buffer := range s.buffers {
			s.consume(buffer)
		}
		return
	case <-finished:
		return
	}
}

func (s *Sender) consume(batch *buffer.BatchBuffer) {
	if batch.Len() == 0 {
		return
	}
	log.GetLogger().WithFields(logrus.Fields{
		"pipe":   s.config.PipeName,
		"offset": batch.Last(),
		"size":   batch.Len(),
	}).Info("sender module is flushing a new batch buffer.")
	packets := make(map[string]otus.BatchePacket)
	for i := 0; i < batch.Len(); i++ {
		packetContext := batch.Buf()[i]
		for _, p := range packetContext.Context {
			protocol := string(p.ApplicationProtoType)
			packets[protocol] = append(packets[protocol], p)
		}
	}
	for _, r := range s.reporters {
		for protocol, ps := range packets {
			if r.SupportProtocol() != protocol {
				continue
			}
			if err := r.Report(ps); err == nil {
				// TODO 发送成功要统计
				continue
			} else {
				log.GetLogger().WithFields(logrus.Fields{
					"pipe":   s.config.PipeName,
					"offset": batch.Last(),
					"size":   batch.Len(),
				}).Warnf("report packet failure: %v", err)
			}
			if !s.fallbacker.Fallback(&ps, r.Report) {
				// TODO 记录失败
			}
		}
	}
}

func (s *Sender) SetInputChannel(partition int, ch <-chan *otus.OutputPacketContext) error {
	if partition < 0 || partition >= len(s.inputs) {
		return fmt.Errorf("invalid partition: %d", partition)
	}
	s.inputs[partition] = ch
	log.GetLogger().Infof("set channel for partition: %d", partition)
	return nil
}

func (s *Sender) IsChannelSet(partition int) bool {
	return s.inputs[partition] != nil
}
