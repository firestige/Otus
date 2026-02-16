package models

import (
	"net/netip"
	"testing"
	"time"

	"firestige.xyz/otus/internal/core"
)

// Test that re-exported types match core types
func TestTypeAliases(t *testing.T) {
	t.Run("RawPacket", func(t *testing.T) {
		// Create using models alias
		var modelsRaw RawPacket
		modelsRaw.Data = []byte{0x01, 0x02}
		modelsRaw.Timestamp = time.Now()

		// Should be assignable to core type
		var coreRaw core.RawPacket = modelsRaw

		if len(coreRaw.Data) != 2 {
			t.Errorf("expected Data length 2, got %d", len(coreRaw.Data))
		}
	})

	t.Run("DecodedPacket", func(t *testing.T) {
		var modelsDecoded DecodedPacket
		modelsDecoded.Reassembled = true

		var coreDecoded core.DecodedPacket = modelsDecoded

		if !coreDecoded.Reassembled {
			t.Error("expected Reassembled=true")
		}
	})

	t.Run("OutputPacket", func(t *testing.T) {
		var modelsOutput OutputPacket
		modelsOutput.TaskID = "task-001"
		modelsOutput.SrcIP = netip.MustParseAddr("192.168.1.1")

		var coreOutput core.OutputPacket = modelsOutput

		if coreOutput.TaskID != "task-001" {
			t.Errorf("expected TaskID=task-001, got %s", coreOutput.TaskID)
		}
		if coreOutput.SrcIP.String() != "192.168.1.1" {
			t.Errorf("expected SrcIP=192.168.1.1, got %s", coreOutput.SrcIP)
		}
	})

	t.Run("Labels", func(t *testing.T) {
		modelsLabels := make(Labels)
		modelsLabels["key"] = "value"

		var coreLabels core.Labels = modelsLabels

		if coreLabels["key"] != "value" {
			t.Errorf("expected value, got %s", coreLabels["key"])
		}
	})
}

// Test that types can be used interchangeably
func TestTypeInterchangeability(t *testing.T) {
	t.Run("FunctionAcceptingCoreType", func(t *testing.T) {
		// Function expecting core.RawPacket
		processCoreRaw := func(raw core.RawPacket) int {
			return len(raw.Data)
		}

		// Can pass models.RawPacket
		var modelsRaw RawPacket
		modelsRaw.Data = []byte{0x01, 0x02, 0x03}

		result := processCoreRaw(modelsRaw)
		if result != 3 {
			t.Errorf("expected 3, got %d", result)
		}
	})

	t.Run("FunctionReturningCoreType", func(t *testing.T) {
		// Function returning core.DecodedPacket
		createCoreDecoded := func() core.DecodedPacket {
			return core.DecodedPacket{
				Reassembled: true,
			}
		}

		// Can assign to models.DecodedPacket
		var modelsDecoded DecodedPacket = createCoreDecoded()

		if !modelsDecoded.Reassembled {
			t.Error("expected Reassembled=true")
		}
	})
}

// Test zero values
func TestZeroValues(t *testing.T) {
	t.Run("RawPacketZero", func(t *testing.T) {
		var raw RawPacket
		if raw.Data != nil {
			t.Error("expected nil Data")
		}
		if !raw.Timestamp.IsZero() {
			t.Error("expected zero Timestamp")
		}
	})

	t.Run("DecodedPacketZero", func(t *testing.T) {
		var decoded DecodedPacket
		if decoded.Reassembled {
			t.Error("expected Reassembled=false")
		}
		if decoded.Payload != nil {
			t.Error("expected nil Payload")
		}
	})

	t.Run("OutputPacketZero", func(t *testing.T) {
		var output OutputPacket
		if output.TaskID != "" {
			t.Errorf("expected empty TaskID, got %s", output.TaskID)
		}
		if output.Labels != nil {
			t.Error("expected nil Labels")
		}
	})

	t.Run("LabelsZero", func(t *testing.T) {
		var labels Labels
		// Nil map should not panic on read
		if val := labels["nonexistent"]; val != "" {
			t.Errorf("expected empty string, got %s", val)
		}
	})
}
