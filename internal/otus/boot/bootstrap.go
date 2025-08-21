package boot

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"
	"time"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/internal/otus/config"
	"firestige.xyz/otus/internal/otus/metrics"
	"firestige.xyz/otus/internal/otus/module/api"
	"firestige.xyz/otus/internal/otus/module/capture"
	"firestige.xyz/otus/internal/otus/module/sender"
	"firestige.xyz/otus/internal/otus/sharable"
	"firestige.xyz/otus/internal/processor"
	plugin "firestige.xyz/otus/plugins"
)

type container map[string][]api.Module

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

	if modules, err := initModules(cfg); err != nil {
		return err
	} else if err := prepareModules(ctx, modules); err != nil {
		return err
	} else if err := sharable.Start(); err != nil {
		return err
	} else {
		bootModules(ctx, modules)
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

func initModules(cfg *config.OtusConfig) (container, error) {
	log.GetLogger().Info("otus is initializing...")
	for _, aCfg := range cfg.Pipes {
		if aCfg.Capture == nil || aCfg.Processor == nil || aCfg.Sender == nil {
			return nil, fmt.Errorf("pipe configuration is incomplete, capture, processor and sender is required")
		}
	}
	container := make(container)
	for _, aCfg := range cfg.Pipes {
		var modules []api.Module
		c := capture.NewCapture(aCfg.Capture)
		p := processor.NewProcessor(aCfg.Processor)
		s := sender.NewSender(aCfg.Sender)
		if err := p.SetCapture(c); err != nil {
			return nil, err
		}
		if err := p.SetSender(s); err != nil {
			return nil, err
		}
		modules = append(modules, c, p, s)
		container[aCfg.CommonConfig.PipeName] = modules
	}
	return container, nil
}

func prepareModules(ctx context.Context, modules container) error {
	log.GetLogger().Info("otus is prepare to start")
	preparedMods := make([]api.Module, 0)
	for ns, mods := range modules {
		for _, mod := range mods {
			preparedMods = append(preparedMods, mod)
			if err := mod.PostConstruct(); err != nil {
				for _, mod := range preparedMods {
					mod.Shutdown()
				}
				log.GetLogger().
					WithField("pipe", ns).
					WithField("module", reflect.TypeOf(mod).String()).
					Errorf("failed to post-construct module: %v", err)
				return err
			}
		}
	}
	return nil
}

func bootModules(ctx context.Context, modules container) {
	log.GetLogger().Info("otus is starting...")
	wg := &sync.WaitGroup{}
	for _, mods := range modules {
		wg.Add(len(mods))
		for _, mod := range mods {
			go func(m api.Module) {
				defer wg.Done()
				m.Boot(ctx)
			}(mod)
		}
	}
	wg.Wait()
}
