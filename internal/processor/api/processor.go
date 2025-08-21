package api

import (
	"firestige.xyz/otus/internal/otus/module/api"
)

type Processor interface {
	api.Module
	SetCapture(m api.Module) error
	SetSender(m api.Module) error
}
