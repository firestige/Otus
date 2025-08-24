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
	sender "firestige.xyz/otus/internal/otus/module/sender/api"
)

type Pipeline interface {
	module.Module
	SetCapture(capture capture.Capture)
	SetSender(sender sender.Sender)
	CreateChannels(bufferSize int) error
}

type Config struct {
	*config.CommonFields

	CaptureConfig *capture.Config `mapstructure:"capture"`
	SenderConfig  *sender.Config  `mapstructure:"sender"`

	BufferSize int    `mapstructure:"buffer_size"`
	Partitions int    `mapstructure:"partitions"`
	FanoutID   uint16 `mapstructure:"fanout_id"`
}

type pipe struct {
	config *Config

	capture  capture.Capture
	sender   sender.Sender
	channels []chan *otus.OutputPacketContext

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

func (p *pipe) CreateChannels(bufferSize int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.capture == nil || p.sender == nil {
		return fmt.Errorf("module not set")
	}

	partitions := p.config.Partitions
	if partitions <= 0 {
		return fmt.Errorf("invalid partitions: %d", partitions)

	}

	p.channels = make([]chan *otus.OutputPacketContext, partitions)
	for i := 0; i < partitions; i++ {
		ch := make(chan *otus.OutputPacketContext, bufferSize)
		p.channels[i] = ch

		if err := p.sender.SetInputChannel(i, ch); err != nil {
			return fmt.Errorf("failed to set input channel for partition %d: %w", i, err)
		}

		if err := p.capture.SetOutputChannel(i, ch); err != nil {
			return fmt.Errorf("failed to set output channel for partition %d: %w", i, err)
		}
	}
	return nil
}

func (p *pipe) PostConstruct() error {
	p.CreateChannels(p.config.BufferSize)
	return nil
}

func (p *pipe) Boot(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isRunning {
		log.GetLogger().WithField("pipe", p.config.PipeName).Warn("pipeline already running")
		return
	}

	if len(p.channels) == 0 {
		log.GetLogger().WithField("pipe", p.config.PipeName).Error("channels not created")
		return
	}

	for i := 0; i < p.config.Partitions; i++ {
		if !p.capture.IsChannelSet(i) {
			log.GetLogger().WithField("pipe", p.config.PipeName).WithField("partition", i).Error("capture output channel not set")
			return
		}
		if !p.sender.IsChannelSet(i) {
			log.GetLogger().WithField("pipe", p.config.PipeName).WithField("partition", i).Error("sender input channel not set")
			return
		}
	}

	p.ctx, p.cancel = context.WithCancel(ctx)
	p.isRunning = true

	// go p.monitorStats()

	p.sender.Boot(p.ctx)
	p.capture.Boot(p.ctx)
}

func (p *pipe) Shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.isRunning {
		log.GetLogger().WithField("pipe", p.config.PipeName).Warn("pipeline not running")
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
		log.GetLogger().WithField("pipe", p.config.PipeName).WithField("partition", i).Info("closed channel")
	}
	p.channels = nil
}
