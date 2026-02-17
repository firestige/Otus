// Package task implements task lifecycle management.
package task

import (
	"sync/atomic"

	"firestige.xyz/otus/internal/core"
)

// DispatchStrategy determines how packets are distributed across pipelines.
type DispatchStrategy interface {
	// Dispatch returns the pipeline index (0-based) for the given packet.
	// numPipelines is guaranteed to be > 0.
	Dispatch(pkt core.RawPacket, numPipelines int) int

	// Name returns the strategy name for logging/metrics.
	Name() string
}

// FlowHashStrategy distributes packets by flow-hash (5-tuple FNV-1a).
// Same flow always goes to the same pipeline (flow affinity).
type FlowHashStrategy struct{}

func (s *FlowHashStrategy) Dispatch(pkt core.RawPacket, numPipelines int) int {
	return int(flowHash(pkt) % uint32(numPipelines))
}

func (s *FlowHashStrategy) Name() string { return "flow-hash" }

// RoundRobinStrategy distributes packets in round-robin order.
// Provides even load distribution but no flow affinity.
type RoundRobinStrategy struct {
	counter atomic.Uint64
}

func (s *RoundRobinStrategy) Dispatch(_ core.RawPacket, numPipelines int) int {
	return int(s.counter.Add(1) % uint64(numPipelines))
}

func (s *RoundRobinStrategy) Name() string { return "round-robin" }

// NewDispatchStrategy creates a dispatch strategy by name.
// Supported strategies: "flow-hash" (default), "round-robin".
func NewDispatchStrategy(name string) DispatchStrategy {
	switch name {
	case "round-robin":
		return &RoundRobinStrategy{}
	default:
		return &FlowHashStrategy{}
	}
}
