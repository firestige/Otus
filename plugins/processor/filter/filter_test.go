package filter

import (
	"testing"

	"icc.tech/capture-agent/internal/core"
)

// helper builds an OutputPacket with the given PayloadType and, optionally,
// a sip.method label.
func sipPkt(method string) *core.OutputPacket {
	pkt := &core.OutputPacket{
		PayloadType: "sip",
		Labels:      core.Labels{},
	}
	if method != "" {
		pkt.Labels[core.LabelSIPMethod] = method
	}
	return pkt
}

func rtpPkt() *core.OutputPacket {
	return &core.OutputPacket{PayloadType: "rtp", Labels: core.Labels{}}
}

func rawPkt() *core.OutputPacket {
	return &core.OutputPacket{PayloadType: "raw", Labels: core.Labels{}}
}

// ── drop_raw ─────────────────────────────────────────────────────────────────

func TestDropRaw(t *testing.T) {
	f := &FilterProcessor{dropRaw: true}
	if f.Process(rawPkt()) {
		t.Error("expected raw packet to be dropped")
	}
	if !f.Process(rtpPkt()) {
		t.Error("expected rtp packet to pass")
	}
	if !f.Process(sipPkt("INVITE")) {
		t.Error("expected sip packet to pass when only drop_raw is set")
	}
}

// ── sip_deny_methods ─────────────────────────────────────────────────────────

func TestSIPDenyMethods_blocksListed(t *testing.T) {
	f := &FilterProcessor{
		sipDenySet: map[string]struct{}{"OPTIONS": {}, "NOTIFY": {}},
	}
	if f.Process(sipPkt("OPTIONS")) {
		t.Error("OPTIONS should be dropped")
	}
	if f.Process(sipPkt("NOTIFY")) {
		t.Error("NOTIFY should be dropped")
	}
}

func TestSIPDenyMethods_passesOthers(t *testing.T) {
	f := &FilterProcessor{
		sipDenySet: map[string]struct{}{"OPTIONS": {}},
	}
	if !f.Process(sipPkt("INVITE")) {
		t.Error("INVITE should pass")
	}
}

func TestSIPDenyMethods_caseInsensitiveInput(t *testing.T) {
	// Init() normalises to uppercase; simulate that.
	set, _ := parseMethodSet([]any{"options", "Notify"})
	f := &FilterProcessor{sipDenySet: set}
	if f.Process(sipPkt("OPTIONS")) {
		t.Error("OPTIONS should be dropped regardless of input case")
	}
	// packet label is also normalised in Process()
	if f.Process(&core.OutputPacket{
		PayloadType: "sip",
		Labels:      core.Labels{core.LabelSIPMethod: "options"}, // lowercase label
	}) {
		t.Error("options (lowercase label) should be dropped")
	}
}

func TestSIPDenyMethods_passesSIPResponse(t *testing.T) {
	f := &FilterProcessor{
		sipDenySet: map[string]struct{}{"INVITE": {}},
	}
	// SIP response: method label is empty
	if !f.Process(sipPkt("")) {
		t.Error("SIP response (no method label) should always pass")
	}
}

func TestSIPDenyMethods_passesNonSIP(t *testing.T) {
	f := &FilterProcessor{
		sipDenySet: map[string]struct{}{"OPTIONS": {}},
	}
	if !f.Process(rtpPkt()) {
		t.Error("RTP packet should be unaffected by sip_deny_methods")
	}
}

// ── sip_allow_methods ────────────────────────────────────────────────────────

func TestSIPAllowMethods_blocksUnlisted(t *testing.T) {
	f := &FilterProcessor{
		sipAllowSet: map[string]struct{}{"INVITE": {}, "BYE": {}, "ACK": {}},
	}
	if f.Process(sipPkt("OPTIONS")) {
		t.Error("OPTIONS not in allowlist, should be dropped")
	}
	if f.Process(sipPkt("REGISTER")) {
		t.Error("REGISTER not in allowlist, should be dropped")
	}
}

func TestSIPAllowMethods_passesListed(t *testing.T) {
	f := &FilterProcessor{
		sipAllowSet: map[string]struct{}{"INVITE": {}, "BYE": {}, "ACK": {}},
	}
	if !f.Process(sipPkt("INVITE")) {
		t.Error("INVITE is in allowlist, should pass")
	}
	if !f.Process(sipPkt("BYE")) {
		t.Error("BYE is in allowlist, should pass")
	}
}

func TestSIPAllowMethods_passesSIPResponse(t *testing.T) {
	f := &FilterProcessor{
		sipAllowSet: map[string]struct{}{"INVITE": {}, "BYE": {}},
	}
	if !f.Process(sipPkt("")) {
		t.Error("SIP response (no method label) should always pass")
	}
}

func TestSIPAllowMethods_passesNonSIP(t *testing.T) {
	f := &FilterProcessor{
		sipAllowSet: map[string]struct{}{"INVITE": {}},
	}
	if !f.Process(rtpPkt()) {
		t.Error("RTP should be unaffected by sip_allow_methods")
	}
}

// ── deny + allow together (deny wins) ────────────────────────────────────────

func TestDenyTakesPrecedenceOverAllow(t *testing.T) {
	f := &FilterProcessor{
		sipDenySet:  map[string]struct{}{"INVITE": {}},
		sipAllowSet: map[string]struct{}{"INVITE": {}, "BYE": {}},
	}
	// INVITE is in both: deny wins
	if f.Process(sipPkt("INVITE")) {
		t.Error("INVITE denied by deny list despite being in allow list")
	}
	// BYE is only in allow list: passes
	if !f.Process(sipPkt("BYE")) {
		t.Error("BYE should pass (allow list)")
	}
}

// ── Init() parsing ────────────────────────────────────────────────────────────

func TestInit_parsesDenyAndAllow(t *testing.T) {
	f := NewFilterProcessor().(*FilterProcessor)
	err := f.Init(map[string]any{
		"sip_deny_methods":  []any{"OPTIONS", "NOTIFY"},
		"sip_allow_methods": []any{"INVITE", "BYE"},
	})
	if err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	if _, ok := f.sipDenySet["OPTIONS"]; !ok {
		t.Error("sipDenySet missing OPTIONS")
	}
	if _, ok := f.sipAllowSet["INVITE"]; !ok {
		t.Error("sipAllowSet missing INVITE")
	}
}

func TestInit_emptyListLeavesNil(t *testing.T) {
	f := NewFilterProcessor().(*FilterProcessor)
	if err := f.Init(map[string]any{
		"sip_deny_methods":  []any{},
		"sip_allow_methods": []any{},
	}); err != nil {
		t.Fatalf("Init error: %v", err)
	}
	if f.sipDenySet != nil {
		t.Error("empty sip_deny_methods should leave sipDenySet nil")
	}
	if f.sipAllowSet != nil {
		t.Error("empty sip_allow_methods should leave sipAllowSet nil")
	}
}

// ── Stats counters ────────────────────────────────────────────────────────────

func TestStats_allCounterPaths(t *testing.T) {
	f := NewFilterProcessor().(*FilterProcessor)
	_ = f.Init(map[string]any{
		"drop_raw":          true,
		"drop_hep":          true,
		"sip_deny_methods":  []any{"OPTIONS"},
		"sip_allow_methods": []any{"INVITE", "BYE", "ACK"},
	})

	// HEP packet — drop_hep fires
	hepPkt := &core.OutputPacket{
		PayloadType: "raw",
		RawPayload:  []byte{'H', 'E', 'P', '3', 0x00},
		Labels:      core.Labels{},
	}
	if f.Process(hepPkt) {
		t.Error("HEP packet should be dropped")
	}

	// raw packet — drop_raw fires
	if f.Process(rawPkt()) {
		t.Error("raw packet should be dropped")
	}

	// SIP OPTIONS — sip_deny fires
	if f.Process(sipPkt("OPTIONS")) {
		t.Error("OPTIONS should be dropped by deny list")
	}

	// SIP REGISTER — sip_allow fires (not in allowlist)
	if f.Process(sipPkt("REGISTER")) {
		t.Error("REGISTER should be dropped by allow list")
	}

	// SIP INVITE — passes both filters
	if !f.Process(sipPkt("INVITE")) {
		t.Error("INVITE should pass")
	}

	// RTP — unaffected, passes
	if !f.Process(rtpPkt()) {
		t.Error("RTP should pass")
	}

	s := f.ReadStats()
	if s.DropHEP != 1 {
		t.Errorf("DropHEP = %d, want 1", s.DropHEP)
	}
	if s.DropRaw != 1 {
		t.Errorf("DropRaw = %d, want 1", s.DropRaw)
	}
	if s.DropDeny != 1 {
		t.Errorf("DropDeny = %d, want 1", s.DropDeny)
	}
	if s.DropAllow != 1 {
		t.Errorf("DropAllow = %d, want 1", s.DropAllow)
	}
	if s.Passed != 2 {
		t.Errorf("Passed = %d, want 2", s.Passed)
	}
	total := s.Passed + s.DropHEP + s.DropRaw + s.DropDeny + s.DropAllow
	if total != 6 {
		t.Errorf("total = %d, want 6", total)
	}
}

func TestStats_logStatsDoesNotPanic(t *testing.T) {
	f := NewFilterProcessor().(*FilterProcessor)
	_ = f.Init(map[string]any{"drop_raw": true})
	f.Process(rawPkt())
	f.Process(rtpPkt())
	// Must not panic; output is a side-effect (slog).
	f.LogStats("task-test", 0)
}
