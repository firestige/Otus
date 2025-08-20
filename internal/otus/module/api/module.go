package api

import "context"

type Module interface {
	PostConstruct() error
	Boot(ctx context.Context)
	Shutdown()
}
