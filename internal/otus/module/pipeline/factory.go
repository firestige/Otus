package pipeline

import (
	"context"
	"sync"

	"firestige.xyz/otus/internal/otus/module/capture"
	"firestige.xyz/otus/internal/otus/module/sender"
)

// pipline作为实际装配，组织和控制的模块，承担了构造模块与依赖注入的角色
func NewPipeline(ctx context.Context, cfg *Config) Pipeline {
	ctx, cancel := context.WithCancel(ctx)
	pipe := &pipe{
		config: cfg,
		mu:     &sync.RWMutex{},
		ctx:    ctx,
		cancel: cancel,
	}

	// 使用 json.MarshalIndent 递归打印（注意：仅导出字段会被序列化）
	// if b, err := json.MarshalIndent(cfg.CaptureConfig, "", "  "); err == nil {
	// 	log.GetLogger().Infof("capture config: %s", string(b))
	// } else {
	// 	log.GetLogger().WithField("err", err).Infof("capture config marshal error")
	// }

	capture := capture.NewCapture(ctx, cfg.CaptureConfig)
	sender := sender.NewSender(ctx, cfg.SenderConfig)

	pipe.SetCapture(capture)
	pipe.SetSender(sender)

	return pipe
}
