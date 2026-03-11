// Package filter implements a configurable drop-rule processor.
//
// Supported config keys:
//
//	drop_hep (bool, default false) — drop any UDP packet whose application-layer
//	  payload starts with the HEPv3 magic bytes "HEP3" (0x48 0x45 0x50 0x33).
//	  This prevents self-captured HEP frames — sent by the agent's own HEP
//	  reporter and visible on the same AF_PACKET socket — from cycling through
//	  the pipeline and filling the reporter queue again.
//
//	drop_raw (bool, default false) — drop any packet whose PayloadType is "raw",
//	  i.e. no configured parser (SIP, RTP, …) successfully recognised the payload.
//	  Prevents unrecognised UDP traffic captured by a wide BPF port-range filter
//	  from being forwarded to reporters and overwhelming downstream collectors.
package filter

import (
	"context"

	"icc.tech/capture-agent/internal/core"
	"icc.tech/capture-agent/pkg/plugin"
)

// hepMagic is the four-byte prefix of every HEPv3 frame.
var hepMagic = [4]byte{'H', 'E', 'P', '3'}

// FilterProcessor drops packets matching configured rules.
type FilterProcessor struct {
	dropHEP bool
	dropRaw bool
}

// NewFilterProcessor creates a new FilterProcessor instance.
func NewFilterProcessor() plugin.Processor {
	return &FilterProcessor{}
}

// Name returns the plugin identifier.
func (f *FilterProcessor) Name() string { return "filter" }

// Init reads configuration.
func (f *FilterProcessor) Init(cfg map[string]any) error {
	if v, ok := cfg["drop_hep"].(bool); ok {
		f.dropHEP = v
	}
	if v, ok := cfg["drop_raw"].(bool); ok {
		f.dropRaw = v
	}
	return nil
}

// Start is a no-op.
func (f *FilterProcessor) Start(_ context.Context) error { return nil }

// Stop is a no-op.
func (f *FilterProcessor) Stop(_ context.Context) error { return nil }

// Process returns false (drop) when any configured rule matches, true otherwise.
func (f *FilterProcessor) Process(pkt *core.OutputPacket) bool {
	if f.dropHEP && len(pkt.RawPayload) >= 4 {
		if pkt.RawPayload[0] == hepMagic[0] &&
			pkt.RawPayload[1] == hepMagic[1] &&
			pkt.RawPayload[2] == hepMagic[2] &&
			pkt.RawPayload[3] == hepMagic[3] {
			return false
		}
	}
	if f.dropRaw && pkt.PayloadType == "raw" {
		return false
	}
	return true
}
