package sender

import "firestige.xyz/otus/internal/otus/module/sender/api"

func NewSender(cfg *api.Config) api.Sender {
	return defaultSender{}
}
