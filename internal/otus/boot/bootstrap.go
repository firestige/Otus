package boot

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/internal/otus/config"
	"firestige.xyz/otus/internal/otus/metrics"
	plugin "firestige.xyz/otus/plugins"
)

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
	defer sharable.Shutdown()

	// app := otus.GetAppContext()
	// plugin.SeekAndRegisterModules()
	// app.SeekAndRegisterModules()
	// app.BuildComponents()
	// defer app.Shutdown()
	// if err := app.StartComponents(); err != nil {
	// 	return err
	// }
	// return nil
}

func initShutdownListener(cancel context.CancelFunc) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-signals
		cancel()
	}()
}
