package boot

import (
	"time"

	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/internal/otus"
	"firestige.xyz/otus/plugin"
)

func Start(cfg *config.OtusConfig, timeout time.Duration) error {
	log.Init(cfg.Logger)

	app := otus.GetAppContext()
	plugin.SeekAndRegisterModules()
	app.SeekAndRegisterModules()
	app.BuildComponents(cfg)
	defer app.Shutdown()
	if err := app.StartComponents(); err != nil {
		return err
	}
	return nil
}
