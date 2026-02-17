// Package plugins registers all built-in plugins.
package plugins

import (
	"firestige.xyz/otus/pkg/plugin"
	"firestige.xyz/otus/plugins/capture/afpacket"
	"firestige.xyz/otus/plugins/parser/sip"
)

func init() {
	// Register capture plugins
	plugin.RegisterCapturer("afpacket", afpacket.NewAFPacketCapturer)

	// Register parser plugins
	plugin.RegisterParser("sip", sip.NewSIPParser)

	// More plugins will be registered here as they are implemented
	// processor plugins
	// reporter plugins
}
