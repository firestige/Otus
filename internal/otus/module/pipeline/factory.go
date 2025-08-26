package pipeline

import (
	"encoding/json"
	"sync"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/internal/otus/module/capture"
	"firestige.xyz/otus/internal/otus/module/sender"
)

func NewPipeline(cfg *Config) Pipeline {
	pipe := &pipe{
		config: cfg,
		mu:     &sync.RWMutex{},
	}

	// 使用 json.MarshalIndent 递归打印（注意：仅导出字段会被序列化）
	if b, err := json.MarshalIndent(cfg.CaptureConfig, "", "  "); err == nil {
		log.GetLogger().Infof("capture config: %s", string(b))
	} else {
		log.GetLogger().WithField("err", err).Infof("capture config marshal error")
	}

	capture := capture.NewCapture(cfg.CaptureConfig)
	sender := sender.NewSender(cfg.SenderConfig)

	pipe.SetCapture(capture)
	pipe.SetSender(sender)

	pipe.PostConstruct()

	return pipe
}
