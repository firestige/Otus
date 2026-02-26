// Package hep implements a HEPv3 UDP reporter plugin.
//
// Each OutputPacket is encoded as a HEPv3 frame (see encoder.go) and sent over
// UDP to one of the configured remote capture servers.  Routing is flow-stable:
// the target server is selected by hashing the 5-tuple (srcIP, srcPort, dstIP,
// dstPort, protocol) modulo len(servers), so all packets from the same network
// flow always reach the same server — important for session correlation in tools
// like Homer/Sipcapture.
//
// Example task reporter configuration:
//
//	reporters:
//	  - type: hep
//	    servers:
//	      - "10.0.0.1:9060"
//	      - "10.0.0.2:9060"
//	    capture_id: 2001
//	    auth_key:   "mysecret"   # optional
package hep

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net"
	"sync/atomic"

	"firestige.xyz/otus/internal/core"
	"firestige.xyz/otus/pkg/plugin"
)

// ─── Reporter ──────────────────────────────────────────────────────────────

// HEPReporter sends OutputPackets as HEPv3 frames via UDP.
type HEPReporter struct {
	name   string
	config Config

	// One pre-dialed UDP connection per configured server.
	// Connections are created in Start() and closed in Stop().
	conns []*net.UDPConn

	// Statistics (exported via metrics if wired up in the future).
	sentCount  atomic.Uint64
	errorCount atomic.Uint64
}

// Config holds HEP reporter configuration.
type Config struct {
	// Servers lists remote UDP endpoints (host:port) to forward HEP frames to.
	// Routing is flow-stable: same 5-tuple always hits the same server.
	// At least one server is required.
	Servers []string `json:"servers"`

	// CaptureID is placed in HEP chunk 12 to identify this agent on the collector side.
	// Default: 0.
	CaptureID uint32 `json:"capture_id"`

	// AuthKey is an optional authentication key written into HEP chunk 14.
	// Leave empty to omit the chunk.
	AuthKey string `json:"auth_key"`

	// NodeName is the capture node identifier written into HEP chunk 19.
	// Typically set to the hostname or datacenter label of this agent.
	// Leave empty to omit the chunk.
	NodeName string `json:"node_name"`
}

// ─── Constructor ───────────────────────────────────────────────────────────

// NewHEPReporter creates a new HEP reporter instance.
func NewHEPReporter() plugin.Reporter {
	return &HEPReporter{name: "hep"}
}

// ─── Plugin interface ──────────────────────────────────────────────────────

// Name returns the plugin identifier.
func (r *HEPReporter) Name() string { return r.name }

// Init validates and applies configuration.
func (r *HEPReporter) Init(config map[string]any) error {
	if config == nil {
		return fmt.Errorf("hep reporter: configuration is required")
	}

	var cfg Config

	// Required: servers
	switch v := config["servers"].(type) {
	case []any:
		cfg.Servers = make([]string, 0, len(v))
		for i, s := range v {
			str, ok := s.(string)
			if !ok {
				return fmt.Errorf("hep reporter: servers[%d] is not a string", i)
			}
			cfg.Servers = append(cfg.Servers, str)
		}
	case []string:
		cfg.Servers = v
	default:
		return fmt.Errorf("hep reporter: servers must be a list of host:port strings")
	}

	if len(cfg.Servers) == 0 {
		return fmt.Errorf("hep reporter: at least one server is required")
	}

	// Optional: capture_id
	switch v := config["capture_id"].(type) {
	case float64:
		cfg.CaptureID = uint32(v)
	case int:
		cfg.CaptureID = uint32(v)
	case uint32:
		cfg.CaptureID = v
	}

	// Optional: auth_key
	if v, ok := config["auth_key"].(string); ok {
		cfg.AuthKey = v
	}

	// Optional: node_name
	if v, ok := config["node_name"].(string); ok {
		cfg.NodeName = v
	}

	r.config = cfg
	return nil
}

// Start opens UDP connections to all configured servers.
func (r *HEPReporter) Start(_ context.Context) error {
	r.conns = make([]*net.UDPConn, 0, len(r.config.Servers))
	for _, srv := range r.config.Servers {
		addr, err := net.ResolveUDPAddr("udp", srv)
		if err != nil {
			r.closeConns() // clean up any already-opened connections
			return fmt.Errorf("hep reporter: resolve %q: %w", srv, err)
		}
		conn, err := net.DialUDP("udp", nil, addr)
		if err != nil {
			r.closeConns()
			return fmt.Errorf("hep reporter: dial %q: %w", srv, err)
		}
		r.conns = append(r.conns, conn)
	}
	slog.Info("hep reporter started",
		"servers", r.config.Servers,
		"capture_id", r.config.CaptureID,
	)
	return nil
}

// Stop closes all UDP connections and logs final statistics.
func (r *HEPReporter) Stop(_ context.Context) error {
	r.closeConns()
	slog.Info("hep reporter stopped",
		"sent", r.sentCount.Load(),
		"errors", r.errorCount.Load(),
	)
	return nil
}

// closeConns closes all open UDP connections, ignoring errors.
func (r *HEPReporter) closeConns() {
	for _, c := range r.conns {
		if c != nil {
			_ = c.Close()
		}
	}
	r.conns = nil
}

// ─── Reporter interface ────────────────────────────────────────────────────

// Report encodes pkt as a HEPv3 frame and sends it to a flow-stable server.
func (r *HEPReporter) Report(_ context.Context, pkt *core.OutputPacket) error {
	if pkt == nil {
		return fmt.Errorf("hep reporter: nil packet")
	}

	frame, err := Encode(pkt, EncodeOptions{
		CaptureID: r.config.CaptureID,
		AuthKey:   r.config.AuthKey,
		NodeName:  r.config.NodeName,
	})
	if err != nil {
		r.errorCount.Add(1)
		return fmt.Errorf("hep reporter: encode: %w", err)
	}

	conn := r.selectConn(pkt)
	if _, err = conn.Write(frame); err != nil {
		r.errorCount.Add(1)
		return fmt.Errorf("hep reporter: send to %s: %w", conn.RemoteAddr(), err)
	}

	r.sentCount.Add(1)
	return nil
}

// Flush is a no-op for the HEP UDP reporter — packets are sent immediately.
func (r *HEPReporter) Flush(_ context.Context) error { return nil }

// ─── Flow-stable routing ───────────────────────────────────────────────────

// selectConn returns the UDP connection for the server that owns pkt's flow.
//
// The mapping is computed as:
//
//	idx = FNV-32a(srcIP‖srcPort‖dstIP‖dstPort‖protocol) % len(conns)
//
// Using FNV-32a (non-cryptographic, fast) is appropriate here — we only need
// uniform distribution and stability, not security.
func (r *HEPReporter) selectConn(pkt *core.OutputPacket) *net.UDPConn {
	if len(r.conns) == 1 {
		return r.conns[0]
	}

	h := fnv.New32a()

	// Write IP bytes — As16() returns a canonical 16-byte form for both
	// IPv4-mapped and native IPv6 addresses, giving consistent hashing.
	src16 := pkt.SrcIP.As16()
	dst16 := pkt.DstIP.As16()
	_, _ = h.Write(src16[:])

	var port [2]byte
	binary.BigEndian.PutUint16(port[:], pkt.SrcPort)
	_, _ = h.Write(port[:])

	_, _ = h.Write(dst16[:])
	binary.BigEndian.PutUint16(port[:], pkt.DstPort)
	_, _ = h.Write(port[:])

	_, _ = h.Write([]byte{pkt.Protocol})

	idx := h.Sum32() % uint32(len(r.conns))
	return r.conns[idx]
}
