package module

import "context"

type Module interface {
	Prepare()
	Boot(ctx context.Context) error
	Shutdown()
}
