// Package plugins registers all built-in plugins.
package plugins

import (
	"firestige.xyz/otus/pkg/plugin"
	"firestige.xyz/otus/plugins/capture/afpacket"
)

func init() {
	// Register capture plugins
	plugin.RegisterCapturer("afpacket", afpacket.NewAFPacketCapturer)

	// More plugins will be registered here as they are implemented
	// parser plugins
	// processor plugins
	// reporter plugins
}
