package utils

import (
	"fmt"

	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"golang.org/x/net/bpf"
)

func CompileBpf(filter string, snapLen int) ([]bpf.RawInstruction, error) {
	pcapBpf, err := pcap.CompileBPFFilter(layers.LinkTypeEthernet, snapLen, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to compile BPF filter: %w", err)
	}

	rawBpf := make([]bpf.RawInstruction, len(pcapBpf))
	for i, ins := range pcapBpf {
		rawBpf[i] = bpf.RawInstruction{Op: ins.Code, Jt: ins.Jt, Jf: ins.Jf, K: ins.K}
	}
	return rawBpf, nil
}
