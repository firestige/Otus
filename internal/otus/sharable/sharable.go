package sharable

import (
	"sync"

	"firestige.xyz/otus/internal/otus/config"
	"firestige.xyz/otus/internal/plugin"
	client "firestige.xyz/otus/plugins/client/api"
)

var (
	Manager map[string]plugin.SharablePlugin
	once    sync.Once
)

func Load(cfg *config.SharableConfig) {
	once.Do(func() {
		Manager = make(map[string]plugin.SharablePlugin)
		for _, c := range cfg.Clients {
			p := client.GetClient(c)
			Manager[p.Name()] = p
		}
	})
}

func PostConstruct() error {
	return nil
}

func Start() error {
	return nil
}

func Close() {}
