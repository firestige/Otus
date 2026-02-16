// Package plugin defines the plugin lifecycle interface.
package plugin

import "context"

// Plugin is the base interface for all plugins.
type Plugin interface {
	Name() string
	Init(cfg map[string]any) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
