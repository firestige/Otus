// Package plugins registers all built-in plugins.
package plugins

import (
	"icc.tech/capture-agent/pkg/plugin"
	"icc.tech/capture-agent/plugins/capture/afpacket"
	"icc.tech/capture-agent/plugins/parser/rtp"
	"icc.tech/capture-agent/plugins/parser/sip"
	"icc.tech/capture-agent/plugins/reporter/console"
	"icc.tech/capture-agent/plugins/reporter/hep"
	"icc.tech/capture-agent/plugins/reporter/kafka"
)

func init() {
	// Register capture plugins
	plugin.RegisterCapturer("afpacket", afpacket.NewAFPacketCapturer)

	// Register parser plugins
	plugin.RegisterParser("sip", sip.NewSIPParser)
	plugin.RegisterParser("rtp", rtp.NewRTPParser)

	// Register reporter plugins
	plugin.RegisterReporter("console", console.NewConsoleReporter)
	plugin.RegisterReporter("hep", hep.NewHEPReporter)
	plugin.RegisterReporter("kafka", kafka.NewKafkaReporter)

	// More plugins will be registered here as they are implemented
	// processor plugins
}
