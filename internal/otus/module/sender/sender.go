package sender

import (
	otus "firestige.xyz/otus/internal/otus/api"
)

type Sender struct {
	// implementation details
	forwarder interface{}
	client    interface{}
}

func (s *Sender) Send(packet *otus.Exchange) error {
	return nil
}
