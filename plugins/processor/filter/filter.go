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
//	sip_deny_methods ([]string) — drop SIP packets whose method is in this list.
//	  Works for both requests (method from Request-Line) and responses (method
//	  from CSeq header, e.g. "1 INVITE" → "INVITE").  Packets with no
//	  sip.method label are not affected.  Evaluated before sip_allow_methods.
//	  Example: ["OPTIONS", "NOTIFY", "REGISTER"].
//
//	sip_allow_methods ([]string) — if non-empty, drop any SIP packet whose
//	  method is NOT in this list.  Works for both requests and responses.
//	  Packets with no sip.method label and non-SIP traffic always pass through.
//	  Example: ["INVITE", "ACK", "BYE", "CANCEL", "PRACK", "UPDATE"].
package filter

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"

	"icc.tech/capture-agent/internal/core"
	"icc.tech/capture-agent/pkg/plugin"
)

// hepMagic is the four-byte prefix of every HEPv3 frame.
var hepMagic = [4]byte{'H', 'E', 'P', '3'}

// FilterProcessor drops packets matching configured rules.
type FilterProcessor struct {
	dropHEP     bool
	dropRaw     bool
	sipDenySet  map[string]struct{} // nil = disabled
	sipAllowSet map[string]struct{} // nil = disabled

	// Stats counters — updated atomically by Process(), read by LogStats().
	statPassed      atomic.Uint64
	statDropHEP     atomic.Uint64
	statDropRaw     atomic.Uint64
	statDropDeny    atomic.Uint64 // dropped by sip_deny_methods
	statDropAllow   atomic.Uint64 // dropped by sip_allow_methods (not in allowlist)
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

// Stats holds a snapshot of counters for external inspection or testing.
type Stats struct {
	Passed    uint64
	DropHEP   uint64
	DropRaw   uint64
	DropDeny  uint64
	DropAllow uint64
}

// ReadStats returns a point-in-time snapshot of filter counters.
func (f *FilterProcessor) ReadStats() Stats {
	return Stats{
		Passed:    f.statPassed.Load(),
		DropHEP:   f.statDropHEP.Load(),
		DropRaw:   f.statDropRaw.Load(),
		DropDeny:  f.statDropDeny.Load(),
		DropAllow: f.statDropAllow.Load(),
	}
}

// LogStats emits a single structured log line with all counters.
// Implements plugin.StatsReporter — called by the pipeline periodically
// and at shutdown for diagnostics.
func (f *FilterProcessor) LogStats(taskID string, pipelineID int) {
	s := f.ReadStats()
	total := s.Passed + s.DropHEP + s.DropRaw + s.DropDeny + s.DropAllow
	dropped := s.DropHEP + s.DropRaw + s.DropDeny + s.DropAllow
	slog.Info("filter stats",
		"task_id", taskID,
		"pipeline_id", pipelineID,
		"total", total,
		"passed", s.Passed,
		"dropped", dropped,
		"drop_hep", s.DropHEP,
		"drop_raw", s.DropRaw,
		"drop_sip_deny", s.DropDeny,
		"drop_sip_allow", s.DropAllow,
	)
}

// Process returns false (drop) when any configured rule matches, true otherwise.
func (f *FilterProcessor) Process(pkt *core.OutputPacket) bool {
	if f.dropHEP && len(pkt.RawPayload) >= 4 {
		if pkt.RawPayload[0] == hepMagic[0] &&
			pkt.RawPayload[1] == hepMagic[1] &&
			pkt.RawPayload[2] == hepMagic[2] &&
			pkt.RawPayload[3] == hepMagic[3] {
			f.statDropHEP.Add(1)
			return false
		}
	}
	if f.dropRaw && pkt.PayloadType == "raw" {
		f.statDropRaw.Add(1)
		return false
	}

	// SIP method filters — apply to any SIP packet that carries a sip.method
	// label.  The SIP parser populates this for both requests (from the
	// Request-Line) and responses (from the CSeq header), so filtering is
	// uniform across the full SIP dialog.
	if pkt.PayloadType == "sip" {
		method := strings.ToUpper(pkt.Labels[core.LabelSIPMethod])
		if method != "" {
			// Deny list: explicit block takes precedence.
			if f.sipDenySet != nil {
				if _, blocked := f.sipDenySet[method]; blocked {
					f.statDropDeny.Add(1)
					return false
				}
			}
			// Allow list: drop anything NOT in the list.
			if f.sipAllowSet != nil {
				if _, allowed := f.sipAllowSet[method]; !allowed {
					f.statDropAllow.Add(1)
					return false
				}
			}
		}
	}

	f.statPassed.Add(1)
	return true
}
