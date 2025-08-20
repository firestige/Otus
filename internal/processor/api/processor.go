package api

import (
	"firestige.xyz/otus/internal/otus/module/api"
)

type Processor interface {
	SetCapture(m *api.Module)
	SetSender(m *api.Module)
}
