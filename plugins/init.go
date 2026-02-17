// Package plugins registers all built-in plugins.
package plugins

import (
	"firestige.xyz/otus/pkg/plugin"
	"firestige.xyz/otus/plugins/capture/afpacket"
	"firestige.xyz/otus/plugins/parser/sip"
	"firestige.xyz/otus/plugins/reporter/console"
	"firestige.xyz/otus/plugins/reporter/kafka"
)

func init() {
	// Register capture plugins
	plugin.RegisterCapturer("afpacket", afpacket.NewAFPacketCapturer)

	// Register parser plugins
	plugin.RegisterParser("sip", sip.NewSIPParser)

	// Register reporter plugins
	plugin.RegisterReporter("console", console.NewConsoleReporter)
	plugin.RegisterReporter("kafka", kafka.NewKafkaReporter)

	// More plugins will be registered here as they are implemented
	// processor plugins
}
