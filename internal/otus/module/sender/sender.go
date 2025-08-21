package sender

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/internal/otus/module/api"
	capture "firestige.xyz/otus/internal/otus/module/capture/api"
	sender "firestige.xyz/otus/internal/otus/module/sender/api"
	"firestige.xyz/otus/internal/otus/msg"
	client "firestige.xyz/otus/plugins/client/api"
	fallbacker "firestige.xyz/otus/plugins/fallbacker/api"
	reporter "firestige.xyz/otus/plugins/reporter/api"
)

type Sender struct {
	config *sender.Config

	reporters  []reporter.Reporter
	fallbacker fallbacker.Fallbacker
	client     client.Client

	inputs       []chan *msg.OutputMessage
	listener     chan client.ClientStatus
	blocking     int32
	shutdownOnce sync.Once
}

func (s *Sender) PostConstruct() error {
	log.GetLogger().WithField("pipe", s.config.PipeName).Info("sender module is preparing...")

	s.client.RegisterListener(s.listener)
	for _, reporter := range s.reporters {
		err := reporter.PostConstruct(s.client.GetConnectedClient())
		if err != nil {
			return err
		}
	}

	s.inputs = make([]chan *msg.OutputMessage, 0)

	return nil
}

func (s *Sender) Boot(ctx context.Context) {
	log.GetLogger().WithField("pipe", s.config.PipeName).Info("sender module is starting...")
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go s.listen(ctx, wg)
	wg.Wait()
}

func (s *Sender) store(ctx context.Context, wg *sync.WaitGroup) {
	// TODO 补完待发流程，
	// 1.当发送速度跟不上上游事件生产速度时，需要暂存
	// 2.当开始关闭流程时需要暂存到 flush 队列
}

func (s *Sender) listen(ctx context.Context, wg *sync.WaitGroup) {
	defer func() {
		wg.Done()
		log.GetLogger().WithField("pipe", s.config.PipeName).Info("listen routine closed")
	}()
	childCtx, _ := context.WithCancel(ctx)
	for {
		select {
		case <-childCtx.Done():
			return
		case status := <-s.listener:
			switch status {
			case client.Connected:
				log.GetLogger().WithField("pipe", s.config.PipeName).Info("client connected")
				atomic.StoreInt32(&s.blocking, 0)
			case client.Disconnect:
				log.GetLogger().WithField("pipe", s.config.PipeName).Info("client disconnected")
				atomic.StoreInt32(&s.blocking, 1)
			}
		}
	}
}

func (s *Sender) flush(ctx context.Context, wg *sync.WaitGroup) {
	defer func() {
		wg.Done()
		log.GetLogger().WithField("pipe", s.config.PipeName).Info("flush routine closed")
	}()
	childCtx, _ := context.WithCancel(ctx)
	for {
		select {
		case <-childCtx.Done():
			s.Shutdown()
			return
		case m := <-s.flushChannel:
			// TODO flushChannel 需要补完
			s.consume(m)
		}
	}
}

func (s *Sender) Shutdown() {
	s.shutdownOnce.Do(func() {
		s.shutdown0()
	})
}

func (s *Sender) shutdown0() {

}

func (s *Sender) consume(batch msg.BatchData) {
}

func (s *Sender) InputNetPacketChannel() chan<- *msg.OutputMessage {
	return s.inputs[0]
}

func (s *Sender) SetCapture(m api.Module) error {
	if c, ok := m.(*capture.Capture); ok {
		s.capture = c
		return nil
	}
	return fmt.Errorf("invalid capture module type: %T", m)
}
