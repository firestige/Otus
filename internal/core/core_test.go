package core

import (
	"errors"
	"net/netip"
	"testing"
	"time"
)

// Test zero values of core structs
func TestStructZeroValues(t *testing.T) {
	t.Run("EthernetHeader", func(t *testing.T) {
		var eth EthernetHeader
		if eth.EtherType != 0 {
			t.Errorf("expected EtherType=0, got %d", eth.EtherType)
		}
		if eth.VLANs != nil {
			t.Errorf("expected VLANs=nil, got %v", eth.VLANs)
		}
	})

	t.Run("IPHeader", func(t *testing.T) {
		var ip IPHeader
		if ip.Version != 0 {
			t.Errorf("expected Version=0, got %d", ip.Version)
		}
		if ip.SrcIP.IsValid() {
			t.Errorf("expected invalid SrcIP, got %v", ip.SrcIP)
		}
		if ip.DstIP.IsValid() {
			t.Errorf("expected invalid DstIP, got %v", ip.DstIP)
		}
	})

	t.Run("TransportHeader", func(t *testing.T) {
		var th TransportHeader
		if th.SrcPort != 0 || th.DstPort != 0 {
			t.Errorf("expected zero ports, got src=%d dst=%d", th.SrcPort, th.DstPort)
		}
	})

	t.Run("RawPacket", func(t *testing.T) {
		var raw RawPacket
		if raw.Data != nil {
			t.Errorf("expected Data=nil, got %v", raw.Data)
		}
		if !raw.Timestamp.IsZero() {
			t.Errorf("expected zero Timestamp, got %v", raw.Timestamp)
		}
	})

	t.Run("DecodedPacket", func(t *testing.T) {
		var decoded DecodedPacket
		if decoded.Reassembled {
			t.Errorf("expected Reassembled=false, got true")
		}
		if decoded.Payload != nil {
			t.Errorf("expected Payload=nil, got %v", decoded.Payload)
		}
	})

	t.Run("OutputPacket", func(t *testing.T) {
		var out OutputPacket
		if out.TaskID != "" {
			t.Errorf("expected empty TaskID, got %q", out.TaskID)
		}
		if out.Labels != nil {
			t.Errorf("expected Labels=nil, got %v", out.Labels)
		}
	})
}

// Test Labels operations
func TestLabels(t *testing.T) {
	t.Run("CreateAndSet", func(t *testing.T) {
		labels := make(Labels)
		labels[LabelSIPMethod] = "INVITE"
		labels[LabelSIPCallID] = "test-call-id"

		if labels[LabelSIPMethod] != "INVITE" {
			t.Errorf("expected INVITE, got %s", labels[LabelSIPMethod])
		}
		if labels[LabelSIPCallID] != "test-call-id" {
			t.Errorf("expected test-call-id, got %s", labels[LabelSIPCallID])
		}
	})

	t.Run("LabelConstants", func(t *testing.T) {
		// Verify label naming convention {protocol}.{field}
		expected := map[string]string{
			LabelSIPMethod:     "sip.method",
			LabelSIPCallID:     "sip.call_id",
			LabelSIPFromURI:    "sip.from_uri",
			LabelSIPToURI:      "sip.to_uri",
			LabelSIPStatusCode: "sip.status_code",
		}

		for constant, expectedName := range expected {
			if constant != expectedName {
				t.Errorf("label constant mismatch: expected %s, got %s", expectedName, constant)
			}
		}
	})

	t.Run("NilLabels", func(t *testing.T) {
		var labels Labels
		// Accessing nil map should not panic, but return zero value
		if val := labels[LabelSIPMethod]; val != "" {
			t.Errorf("expected empty string from nil map, got %s", val)
		}
	})
}

// Test sentinel errors
func TestSentinelErrors(t *testing.T) {
	t.Run("ErrorIdentity", func(t *testing.T) {
		// Sentinel errors should be identifiable with errors.Is
		err := ErrTaskNotFound
		if !errors.Is(err, ErrTaskNotFound) {
			t.Error("errors.Is failed for ErrTaskNotFound")
		}

		err = ErrPacketTooShort
		if !errors.Is(err, ErrPacketTooShort) {
			t.Error("errors.Is failed for ErrPacketTooShort")
		}
	})

	t.Run("ErrorMessages", func(t *testing.T) {
		tests := []struct {
			err     error
			message string
		}{
			{ErrTaskNotFound, "otus: task not found"},
			{ErrTaskAlreadyExists, "otus: task already exists"},
			{ErrPipelineStopped, "otus: pipeline stopped"},
			{ErrPacketTooShort, "otus: packet too short"},
			{ErrReassemblyTimeout, "otus: fragment reassembly timeout"},
			{ErrPluginNotFound, "otus: plugin not found"},
			{ErrConfigInvalid, "otus: invalid configuration"},
			{ErrDaemonNotRunning, "otus: daemon not running"},
		}

		for _, tt := range tests {
			if tt.err.Error() != tt.message {
				t.Errorf("expected error message %q, got %q", tt.message, tt.err.Error())
			}
		}
	})

	t.Run("ErrorWrapping", func(t *testing.T) {
		// Test that sentinel errors can be wrapped and still identified
		wrapped := errors.Join(ErrTaskNotFound, errors.New("additional context"))
		if !errors.Is(wrapped, ErrTaskNotFound) {
			t.Error("errors.Is failed for wrapped error")
		}
	})
}

// Test packet structures with real data
func TestPacketStructures(t *testing.T) {
	t.Run("RawPacket", func(t *testing.T) {
		now := time.Now()
		raw := RawPacket{
			Data:           []byte{0x01, 0x02, 0x03},
			Timestamp:      now,
			CaptureLen:     3,
			OrigLen:        100,
			InterfaceIndex: 1,
		}

		if len(raw.Data) != 3 {
			t.Errorf("expected Data length 3, got %d", len(raw.Data))
		}
		if raw.Timestamp != now {
			t.Errorf("timestamp mismatch")
		}
		if raw.CaptureLen != 3 {
			t.Errorf("expected CaptureLen=3, got %d", raw.CaptureLen)
		}
		if raw.OrigLen != 100 {
			t.Errorf("expected OrigLen=100, got %d", raw.OrigLen)
		}
	})

	t.Run("DecodedPacket", func(t *testing.T) {
		srcIP := netip.MustParseAddr("192.168.1.1")
		dstIP := netip.MustParseAddr("192.168.1.2")

		decoded := DecodedPacket{
			Timestamp: time.Now(),
			Ethernet: EthernetHeader{
				EtherType: 0x0800, // IPv4
			},
			IP: IPHeader{
				Version:  4,
				SrcIP:    srcIP,
				DstIP:    dstIP,
				Protocol: 6, // TCP
			},
			Transport: TransportHeader{
				SrcPort:  5060,
				DstPort:  5060,
				Protocol: 6,
			},
			Payload:     []byte("test payload"),
			Reassembled: false,
		}

		if decoded.IP.SrcIP != srcIP {
			t.Errorf("SrcIP mismatch")
		}
		if decoded.IP.DstIP != dstIP {
			t.Errorf("DstIP mismatch")
		}
		if decoded.Transport.SrcPort != 5060 {
			t.Errorf("expected SrcPort=5060, got %d", decoded.Transport.SrcPort)
		}
	})

	t.Run("OutputPacket", func(t *testing.T) {
		srcIP := netip.MustParseAddr("10.0.0.1")
		dstIP := netip.MustParseAddr("10.0.0.2")

		labels := make(Labels)
		labels[LabelSIPMethod] = "REGISTER"

		out := OutputPacket{
			TaskID:      "task-001",
			AgentID:     "agent-001",
			PipelineID:  1,
			Timestamp:   time.Now(),
			SrcIP:       srcIP,
			DstIP:       dstIP,
			SrcPort:     5060,
			DstPort:     5060,
			Protocol:    17, // UDP
			Labels:      labels,
			PayloadType: "sip",
			Payload:     "SIP message",
			RawPayload:  []byte("raw SIP data"),
		}

		if out.TaskID != "task-001" {
			t.Errorf("expected TaskID=task-001, got %s", out.TaskID)
		}
		if out.Labels[LabelSIPMethod] != "REGISTER" {
			t.Errorf("label mismatch")
		}
		if out.PayloadType != "sip" {
			t.Errorf("expected PayloadType=sip, got %s", out.PayloadType)
		}
	})
}
