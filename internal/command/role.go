// Package command provides SimpleCommand → TaskConfig translation logic.
//
// When a SimpleCommand arrives (via Kafka or UDS), the agent translates it into
// a full TaskConfig using a three-level priority stack:
//
//  1. Built-in role defaults (portRange, protocol)
//  2. Local role config from config.yml (capture.interface, reporters, workers, …)
//  3. SimpleCommand message fields (portRange, protocol — only when non-nil/non-empty)
//
// The BPF filter is always auto-generated from the final portRange + protocol.
// It includes an IP-fragment catch-all rule to avoid dropping non-first fragments
// (which carry no L4 header and would otherwise be missed by port-based filters).
package command

import (
	"fmt"
	"strings"

	"icc.tech/capture-agent/internal/config"
)

// ─── Role constants ────────────────────────────────────────────────────────

const (
	RoleASBC       = "ASBC"
	RoleFS         = "FS"
	RoleKAMAILIO   = "KAMAILIO"
	RoleTRACEMEDIA = "TRACEMEDIA"
	// Simulator roles — used by voip-simulator capture-agent sidecars.
	RoleUAS = "UAS"
	RoleUAC = "UAC"
)

// ─── Built-in defaults ─────────────────────────────────────────────────────

// roleDefault holds the code-level defaults for a role.
type roleDefault struct {
	portRange string
	protocols []string // upper-case: "SIP", "RTP"
	taskID    string   // default task ID for this role
}

var roleDefaults = map[string]roleDefault{
	RoleASBC:       {portRange: "10000-60000", protocols: []string{"SIP", "RTP"}, taskID: "default-asbc"},
	RoleFS:         {portRange: "10000-60000", protocols: []string{"SIP", "RTP"}, taskID: "default-fs"},
	RoleKAMAILIO:   {portRange: "5060-5061", protocols: []string{"SIP"}, taskID: "default-kamailio"},
	RoleTRACEMEDIA: {portRange: "10000-60000", protocols: []string{"RTP"}, taskID: "default-tracemedia"},
	// Simulator roles: SIP port + broad RTP range to cover voip-simulator traffic.
	RoleUAS: {portRange: "5060-15000", protocols: []string{"SIP", "RTP"}, taskID: "default-uas"},
	RoleUAC: {portRange: "5061-15000", protocols: []string{"SIP", "RTP"}, taskID: "default-uac"},
}

// protocolToParser maps protocol names to parser plugin names.
var protocolToParser = map[string]string{
	"SIP": "sip",
	"RTP": "rtp",
}

// ─── BPF filter ────────────────────────────────────────────────────────────

// buildBPFFilter generates a BPF filter that:
//   - matches the L4 protocol(s) + portRange for the configured protocols
//   - always appends an IP-fragment rule so reassembly works correctly
//
// SIP uses both TCP and UDP; RTP uses UDP only.
// If both SIP and RTP are present, the TCP+UDP range covers all traffic.
//
// The IP-fragment rule `ip[6:2] & 0x3fff != 0` matches:
//   - bit 13 (MF flag = 1) → first and middle fragment
//   - bits 0–12 (fragment offset ≠ 0) → non-first fragments (no L4 header)
//
// Without this rule, non-first fragments are silently dropped, causing
// reassembly failure for fragmented SIP messages.
func buildBPFFilter(protocols []string, portRange string) string {
	hasSIP := false
	hasRTP := false
	for _, p := range protocols {
		switch strings.ToUpper(p) {
		case "SIP":
			hasSIP = true
		case "RTP":
			hasRTP = true
		}
	}

	var l4expr string
	switch {
	case hasSIP:
		// SIP runs over TCP and UDP; the portrange covers both SIP and any
		// co-located RTP when both protocols are requested.
		l4expr = fmt.Sprintf("tcp portrange %s or udp portrange %s", portRange, portRange)
	case hasRTP:
		l4expr = fmt.Sprintf("udp portrange %s", portRange)
	default:
		// Unknown protocol combination — emit a conservative pass-all filter
		// rather than silently capturing nothing.
		return "(ip[6:2] & 0x3fff != 0)"
	}

	// Append IP-fragment catch-all.
	return fmt.Sprintf("(%s) or (ip[6:2] & 0x3fff != 0)", l4expr)
}

// ─── Parser list ───────────────────────────────────────────────────────────

// buildParsers returns a parser list derived from the protocol set.
// The parser.Config map is taken from the matching entry in localParsers
// (keyed by plugin name) when available.
func buildParsers(protocols []string, localParsers []config.ParserConfig) []config.ParserConfig {
	// Index local parser configs by name for O(1) lookup.
	localByName := make(map[string]config.ParserConfig, len(localParsers))
	for _, p := range localParsers {
		localByName[p.Name] = p
	}

	seen := make(map[string]bool)
	result := make([]config.ParserConfig, 0, len(protocols))
	for _, proto := range protocols {
		name, ok := protocolToParser[strings.ToUpper(proto)]
		if !ok || seen[name] {
			continue
		}
		seen[name] = true

		pc := config.ParserConfig{Name: name}
		if local, ok := localByName[name]; ok {
			pc.Config = local.Config
		}
		result = append(result, pc)
	}
	return result
}

// ─── Main translation ──────────────────────────────────────────────────────

// BuildTaskConfig translates a SimpleCmdItem plus the agent's local RoleConfig
// into a complete TaskConfig ready for TaskManager.Create().
//
// Priority (low → high):
//  1. Code built-in defaults (portRange, protocol, taskID)
//  2. localRole fields (interface, workers, capture plugin, reporters, …)
//  3. item.PortRange / item.Protocol (only when non-nil / non-empty)
//
// BPF filter and parser list are always regenerated from the resolved
// portRange + protocol; they are never taken verbatim from localRole.
func BuildTaskConfig(item SimpleCmdItem, localRole config.RoleConfig) (config.TaskConfig, error) {
	role := strings.ToUpper(item.Role)
	def, ok := roleDefaults[role]
	if !ok {
		return config.TaskConfig{}, fmt.Errorf("unknown role %q (valid: ASBC, FS, KAMAILIO, TRACEMEDIA)", item.Role)
	}

	// Resolve portRange: message > built-in default.
	portRange := def.portRange
	if item.PortRange != nil && *item.PortRange != "" {
		portRange = *item.PortRange
	}

	// Resolve protocols: message > built-in default.
	protocols := def.protocols
	if len(item.Protocol) > 0 {
		protocols = item.Protocol
	}

	// Start from the local role config as the base template.
	tc := config.TaskConfig{
		ID:         def.taskID,
		Workers:    localRole.Workers,
		Capture:    localRole.Capture, // interface, plugin name, snap_len, etc.
		Decoder:    localRole.Decoder,
		Processors: localRole.Processors,
		Reporters:  localRole.Reporters,
	}

	// Always override BPF filter from resolved portRange + protocol.
	tc.Capture.BPFFilter = buildBPFFilter(protocols, portRange)

	// Build parser list from protocol, merging local parser.Config entries.
	tc.Parsers = buildParsers(protocols, localRole.Parsers)

	return tc, nil
}
