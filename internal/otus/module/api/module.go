package api

import (
	"context"
	"time"
)

var ShutdownHookTime = time.Second * 5

type Module interface {
	PostConstruct() error
	Boot(ctx context.Context)
	Shutdown()
}
