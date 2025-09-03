package pipeline

import (
	"context"
	"fmt"
	"sync"

	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/log"
	otus "firestige.xyz/otus/internal/otus/api"
	module "firestige.xyz/otus/internal/otus/module/api"
	capture "firestige.xyz/otus/internal/otus/module/capture/api"
	processor "firestige.xyz/otus/internal/otus/module/processor/api"
	sender "firestige.xyz/otus/internal/otus/module/sender/api"
)

type Pipeline interface {
	module.Module
	SetCapture(capture capture.Capture)
	SetProcessor(processor processor.Processor)
	SetSender(sender sender.Sender)
}

type Config struct {
	CommonConfig *config.CommonFields `mapstructure:"common_config"`

	CaptureConfig   *capture.Config   `mapstructure:"capture"`
	ProcessorConfig *processor.Config `mapstructure:"processor"`
	SenderConfig    *sender.Config    `mapstructure:"sender"`

	BufferSize int    `mapstructure:"buffer_size"`
	Partitions int    `mapstructure:"partitions"`
	FanoutID   uint16 `mapstructure:"fanout_id"`
}

type pipe struct {
	config *Config

	capture   capture.Capture
	sender    sender.Sender
	processor processor.Processor
	channels  []chan *otus.OutputPacketContext

	ctx    context.Context
	cancel context.CancelFunc

	mu        *sync.RWMutex
	isRunning bool
}

func (p *pipe) SetCapture(capture capture.Capture) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.capture = capture
}

func (p *pipe) SetSender(sender sender.Sender) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sender = sender
}

func (p *pipe) SetProcessor(processor processor.Processor) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.processor = processor
}

func (p *pipe) bindChannels() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.capture == nil || p.sender == nil {
		return fmt.Errorf("module not set")
	}

	partitions := p.config.Partitions
	if partitions <= 0 {
		return fmt.Errorf("invalid partitions: %d", partitions)

	}

	log.GetLogger().Infof("Creating channels with partitions: %d", partitions)
	for i := 0; i < partitions; i++ {
		out := p.processor.GetOutputChannel(i)
		if err := p.sender.SetInputChannel(i, out); err != nil {
			log.GetLogger().Errorf("Failed to set input channel for partition %d: %v", i, err)
			return fmt.Errorf("failed to set input channel for partition %d: %w", i, err)
		}

		in := p.processor.GetInputChannel(i)
		if err := p.capture.SetOutputChannel(i, in); err != nil {
			log.GetLogger().Errorf("Failed to set output channel for partition %d: %v", i, err)
			return fmt.Errorf("failed to set output channel for partition %d: %w", i, err)
		}
	}
	return nil
}

func (p *pipe) PostConstruct() error {
	p.bindChannels()
	p.sender.PostConstruct()
	p.capture.PostConstruct()
	return nil
}

func (p *pipe) Boot() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isRunning {
		log.GetLogger().WithField("pipe", p.config.CommonConfig.PipeName).Warn("pipeline already running")
		return
	}

	if len(p.channels) == 0 {
		log.GetLogger().WithField("pipe", p.config.CommonConfig.PipeName).Error("channels not created")
		return
	}

	for i := 0; i < p.config.Partitions; i++ {
		if !p.capture.IsChannelSet(i) {
			log.GetLogger().WithField("pipe", p.config.CommonConfig.PipeName).WithField("partition", i).Error("capture output channel not set")
			return
		}
		if !p.sender.IsChannelSet(i) {
			log.GetLogger().WithField("pipe", p.config.CommonConfig.PipeName).WithField("partition", i).Error("sender input channel not set")
			return
		}
	}
	p.isRunning = true

	// go p.monitorStats()

	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		p.sender.Boot()
	}()
	go func() {
		defer wg.Done()
		p.capture.Boot()
	}()
	wg.Wait()
}

func (p *pipe) Shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.isRunning {
		log.GetLogger().WithField("pipe", p.config.CommonConfig.PipeName).Warn("pipeline not running")
		return
	}

	p.cancel()
	p.isRunning = false

	// 先关闭生产者
	p.capture.Shutdown()
	// 再关闭消费者
	p.sender.Shutdown()

	// 最后关闭所有通道
	for i, ch := range p.channels {
		close(ch)
		log.GetLogger().WithField("pipe", p.config.CommonConfig.PipeName).WithField("partition", i).Info("closed channel")
	}
	p.channels = nil
}
