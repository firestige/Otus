package otus

import (
	"context"
	"os"
	"os/signal"
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
	ctx       context.Context
	registry  map[string]interface{}
	container map[string]interface{}
}

func newAppContext() *AppContext {
	ctx, cancel := context.WithCancel(context.Background())
	initShutdownListener(cancel)
	return &AppContext{
		ctx:       ctx,
		registry:  make(map[string]interface{}),
		container: make(map[string]interface{}),
	}
}

func (a *AppContext) GetContext() context.Context {
	return a.ctx
}

func (a *AppContext) register(name string, component interface{}) {
	a.registry[name] = component
}

func (a *AppContext) SeekAndRegisterModules() {
	// TODO: Implement module seeking and registration logic
}

func (a *AppContext) BuildComponents(cfg *config.OtusConfig) {

}

func (a *AppContext) StartComponents() error {
	return nil // TODO: Implement component start logic
}

func (a *AppContext) Shutdown() {
}

func initShutdownListener(cancel context.CancelFunc) {
	signals := make(chan os.Signal)
	signal.Notify(signals, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-signals
		cancel()
	}()
}
