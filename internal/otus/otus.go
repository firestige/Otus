package otus

import (
	"context"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"

	"firestige.xyz/otus/internal/config"
)

var (
	otus *AppContext
	once sync.Once
)

func GetAppContext() *AppContext {
	once.Do(func() {
		otus = newAppContext()
	})
	return otus
}

type AppContext struct {
	ctx      context.Context
	registry map[reflect.Type]map[string]reflect.Value
}

func newAppContext() *AppContext {
	ctx, cancel := context.WithCancel(context.Background())
	initShutdownListener(cancel)
	return &AppContext{
		ctx:      ctx,
		registry: make(map[reflect.Type]map[string]reflect.Value),
	}
}

func (a *AppContext) GetContext() context.Context {
	return a.ctx
}

func (a *AppContext) SeekAndRegisterModules() {
	// plugin.SeekAndRegisterModules()
}

func (a *AppContext) BuildComponents(cfg *config.OtusConfig) {

}

func (a *AppContext) StartComponents() error {
	return nil // TODO: Implement component start logic
}

func (a *AppContext) Shutdown() {
}

func initShutdownListener(cancel context.CancelFunc) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-signals
		cancel()
	}()
}
