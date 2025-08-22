package buffer

import (
	"context"
	"time"
)

type LimiterConfig struct {
	FlushTime  int `mapstructure:"flush_time" yaml:"flush_time"`
	LimitCount int `mapstructure:"limit_count" yaml:"limit_count"`
}

type Flusher func()
type Checker func() int

type Limiter struct {
	Config       LimiterConfig
	checker      Checker
	stopChannel  chan struct{}
	flushChannel chan struct{}
}

func NewLimiter(cfg LimiterConfig, checker Checker) *Limiter {
	return &Limiter{
		Config:       cfg,
		checker:      checker,
		stopChannel:  make(chan struct{}),
		flushChannel: make(chan struct{}),
	}
}

func (l *Limiter) Start(ctx context.Context, flush Flusher) {
	go func() {
		childCtx, cancel := context.WithCancel(ctx)
		timer := time.NewTimer(time.Duration(l.Config.FlushTime) * time.Millisecond)

		defer cancel()

		for {
			timer.Reset(time.Duration(l.Config.FlushTime) * time.Millisecond)
			select {
			case <-timer.C:
				flush()
			case <-l.flushChannel:
				flush()
			case <-l.stopChannel:
				flush()
				return
			case <-childCtx.Done():
				flush()
				return
			}
		}
	}()
}

func (l *Limiter) Check() {
	if l.checker() >= l.Config.LimitCount {
		l.flushChannel <- struct{}{}
	}
}

func (l *Limiter) Stop() {
	l.stopChannel <- struct{}{}
}
