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
	ctx          context.Context
	cancel       context.CancelFunc
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

func (s *Sender) Boot() {
	log.GetLogger().WithField("pipe", s.config.PipeName).Info("sender module is starting...")
	wg := &sync.WaitGroup{}
	wg.Add(2 * s.config.Partition)
	for partition := 0; partition < s.config.Partition; partition++ {
		log.GetLogger().WithField("pipe", s.config.PipeName).WithField("partition", partition).Info("starting sender routines...")
		go s.store(partition, wg)
		go s.flush(partition, wg)
	}
	wg.Wait()
	log.GetLogger().Info("Sender closed")
}

func (s *Sender) store(partition int, wg *sync.WaitGroup) {
	// TODO 补完待发流程，
	// 1.当发送速度跟不上上游事件生产速度时，需要暂存
	// 2.当开始关闭流程时需要暂存到 flush 队列
	// 不要试图合并两个 defer，此外由于 LIFO 特性，wg.Done 实际上是后执行的那个
	defer wg.Done()
	defer log.GetLogger().WithField("pipe", s.config.PipeName).WithField("partition", partition).Info("store routine closed")

	log.GetLogger().WithField("pipe", s.config.PipeName).WithField("partition", partition).Info("store routine started")
	childCtx, _ := context.WithCancel(s.ctx)
	flushTime := s.config.FlushInterval
	if flushTime <= 0 {
		flushTime = defaultSenderFlushTime
	}
	log.GetLogger().WithField("pipe", s.config.PipeName).WithField("partition", partition).Infof("using flush time: %d ms", flushTime)
	timeTicker := time.NewTicker(time.Duration(flushTime) * time.Millisecond)
	for {
		log.GetLogger().WithField("pipe", s.config.PipeName).WithField("partition", partition).Info("waiting for new event or flush timer...")
		if atomic.LoadInt32(&s.blocking) == 1 {
			time.Sleep(100 * time.Millisecond)
			log.GetLogger().WithField("pipe", s.config.PipeName).WithField("partition", partition).Warn("client disconnected, blocking...")
			continue
		}
		log.GetLogger().WithField("pipe", s.config.PipeName).WithField("partition", partition).Info("waiting for new event...")
		select {
		case <-childCtx.Done():
			return
		case <-timeTicker.C:
			log.GetLogger().WithField("pipe", s.config.PipeName).WithField("partition", partition).Info("time ticker triggered...")
			//打印当前 chan 的长度
			log.GetLogger().WithField("pipe", s.config.PipeName).WithField("partition", partition).Infof("received new event, current input channel length: %d", len(s.inputs[partition]))
			if s.buffers[partition].Len() >= s.config.MinFlushEvents {
				log.GetLogger().WithField("pipe", s.config.PipeName).WithField("partition", partition).Infof("time ticker triggered, flushing %d events", s.buffers[partition].Len())
				s.flushChannel[partition] <- s.buffers[partition]
				s.buffers[partition] = buffer.NewBatchBuffer(s.config.MaxBufferSize, partition)
			}
		case e := <-s.inputs[partition]:
			//打印当前 chan 的长度
			log.GetLogger().WithField("pipe", s.config.PipeName).WithField("partition", partition).Infof("received new event, current input channel length: %d", len(s.inputs[partition]))
			if e == nil {
				log.GetLogger().WithField("pipe", s.config.PipeName).WithField("partition", partition).Warn("get nil, continue")
				continue
			}
			s.buffers[partition].Add(e)
			if s.buffers[partition].Len() == s.config.MaxBufferSize {
				log.GetLogger().WithField("pipe", s.config.PipeName).WithField("partition", partition).Infof("max buffer size reached, flushing %d events", s.buffers[partition].Len())
				s.flushChannel[partition] <- s.buffers[partition]
				s.buffers[partition] = buffer.NewBatchBuffer(s.config.MaxBufferSize, partition)
			}
		}
	}
}

func (s *Sender) flush(partition int, wg *sync.WaitGroup) {
	defer wg.Done()
	defer log.GetLogger().WithField("pipe", s.config.PipeName).WithField("partition", partition).Info("flush routine closed")
	childCtx, _ := context.WithCancel(s.ctx)
	for {
		select {
		case <-childCtx.Done():
			s.Shutdown()
			return
		case b := <-s.flushChannel[partition]:
			// TODO flushChannel 需要补完
			log.GetLogger().Infof("flushing a new batch buffer with size: %d", b.Len())
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
	packets := make(map[string]otus.BatchPacket)
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
