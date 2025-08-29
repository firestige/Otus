package api

import (
	"time"
)

var ShutdownHookTime = time.Second * 5

type Module interface {
	PostConstruct() error
	Boot()
	Shutdown()
}
