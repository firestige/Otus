package boot

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/internal/otus/config"
	"firestige.xyz/otus/internal/otus/metrics"
	"firestige.xyz/otus/internal/otus/module/pipeline"
	"firestige.xyz/otus/internal/otus/sharable"
	plugin "firestige.xyz/otus/plugins"
)

type container map[string]pipeline.Pipeline

func Start(cfg *config.OtusConfig, timeout time.Duration) error {
	// 初始化全局组件
	log.Init(cfg.Logger)
	if err := metrics.Init(cfg.Metrics); err != nil {
		return err
	}

	plugin.SeekAndRegisterModules()
	ctx, cancel := context.WithCancel(context.Background())
	initShutdownListener(cancel)

	sharable.Load(cfg.Sharable)
	if err := sharable.PostConstruct(ctx); err != nil {
		return fmt.Errorf("failed to post-construct sharable module: %w", err)
	}
	defer sharable.Close()

	if pipes, err := initPipes(cfg); err != nil {
		return err
	} else if err := preparePipes(pipes); err != nil {
		return err
	} else if err := sharable.Start(); err != nil {
		return err
	} else {
		bootPipes(ctx, pipes)
		return nil
	}
}

func initShutdownListener(cancel context.CancelFunc) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-signals
		cancel()
	}()
}

func initPipes(cfg *config.OtusConfig) (container, error) {
	log.GetLogger().Info("otus is initializing...")
	for _, aCfg := range cfg.Pipes {
		if aCfg.CaptureConfig == nil || aCfg.SenderConfig == nil {
			return nil, fmt.Errorf("pipe configuration is incomplete, capture, processor and sender is required")
		}
	}
	container := make(container)
	for _, aCfg := range cfg.Pipes {
		pipe := pipeline.NewPipeline(aCfg)
		container[aCfg.CommonConfig.PipeName] = pipe
	}
	return container, nil
}

func preparePipes(pipes container) error {
	log.GetLogger().Info("otus is prepare to start")
	preparedPipes := make([]pipeline.Pipeline, 0)
	for ns, pipe := range pipes {
		preparedPipes = append(preparedPipes, pipe)
		if err := pipe.PostConstruct(); err != nil {
			for _, p := range preparedPipes {
				p.Shutdown()
			}
			log.GetLogger().
				WithField("pipe", ns).
				Errorf("failed to post-construct module: %v", err)
			return err
		}
	}
	return nil
}

func bootPipes(ctx context.Context, pipes container) {
	log.GetLogger().Info("otus is starting...")
	wg := &sync.WaitGroup{}
	for _, pipe := range pipes {
		wg.Add(1)
		go func(p pipeline.Pipeline) {
			defer wg.Done()
			p.Boot(ctx)
		}(pipe)
	}
	wg.Wait()
}
