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
//
//	sip_deny_methods ([]string) — drop SIP request packets whose method is in
//	  this list.  Non-SIP packets and SIP responses (no method label) are not
//	  affected.  Example: ["OPTIONS", "NOTIFY", "REGISTER"].
//	  Evaluated before sip_allow_methods when both are set.
//
//	sip_allow_methods ([]string) — if non-empty, drop any SIP request packet
//	  whose method is NOT in this list.  Acts as an allowlist.  Non-SIP packets
//	  and SIP responses always pass through.  Example: ["INVITE", "ACK", "BYE",
//	  "CANCEL", "PRACK", "UPDATE"].
package filter

import (
	"context"
	"strings"

	"icc.tech/capture-agent/internal/core"
	"icc.tech/capture-agent/pkg/plugin"
)

// hepMagic is the four-byte prefix of every HEPv3 frame.
var hepMagic = [4]byte{'H', 'E', 'P', '3'}

// FilterProcessor drops packets matching configured rules.
type FilterProcessor struct {
	dropHEP        bool
	dropRaw        bool
	sipDenySet     map[string]struct{} // nil = disabled
	sipAllowSet    map[string]struct{} // nil = disabled
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
	if v, ok := cfg["sip_deny_methods"]; ok {
		set, err := parseMethodSet(v)
		if err != nil {
			return err
		}
		if len(set) > 0 {
			f.sipDenySet = set
		}
	}
	if v, ok := cfg["sip_allow_methods"]; ok {
		set, err := parseMethodSet(v)
		if err != nil {
			return err
		}
		if len(set) > 0 {
			f.sipAllowSet = set
		}
	}
	return nil
}

// parseMethodSet normalises a config value into a set of uppercase SIP method strings.
// Accepts []string, []any (from YAML unmarshalling), or a single string.
func parseMethodSet(v any) (map[string]struct{}, error) {
	set := make(map[string]struct{})
	switch val := v.(type) {
	case []string:
		for _, m := range val {
			set[strings.ToUpper(strings.TrimSpace(m))] = struct{}{}
		}
	case []any:
		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				continue
			}
			set[strings.ToUpper(strings.TrimSpace(s))] = struct{}{}
		}
	case string:
		set[strings.ToUpper(strings.TrimSpace(val))] = struct{}{}
	}
	delete(set, "") // remove any blank entries
	return set, nil
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

	// SIP method filters — only apply to SIP packets that carry a method label.
	// SIP responses (no sip.method label) and non-SIP traffic are not affected.
	if pkt.PayloadType == "sip" {
		method := strings.ToUpper(pkt.Labels[core.LabelSIPMethod])
		if method != "" {
			// Deny list: explicit block takes precedence.
			if f.sipDenySet != nil {
				if _, blocked := f.sipDenySet[method]; blocked {
					return false
				}
			}
			// Allow list: drop anything NOT in the list.
			if f.sipAllowSet != nil {
				if _, allowed := f.sipAllowSet[method]; !allowed {
					return false
				}
			}
		}
	}

	return true
}
