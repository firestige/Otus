package otus

import "context"

type WithContext interface {
	SetContext(ctx context.Context)
}

type Configurable interface {
	Load(cfg interface{}) error
	ApplyConfig(cfg interface{}) error
}
