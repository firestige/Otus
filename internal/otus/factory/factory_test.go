package factory_test

import (
	"fmt"
	"testing"

	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/factory"
	_ "firestige.xyz/otus/internal/sink/console"
	_ "firestige.xyz/otus/internal/source/afpacket"
	"firestige.xyz/otus/internal/source/file"
	_ "firestige.xyz/otus/internal/source/file"
)

func TestRegistryInit(t *testing.T) {
	cfg := &file.FileCfg{}
	cfg.Name = "file" // 利用字段提升
	cfg.FilePath = "testdata/sample.pcap"
	// factory.GetSource(cfg)
	// if s == nil {
	// 	t.Error("failed to get source")
	// }
	reg := factory.GetRegistry()
	fmt.Printf("%d", len(reg))
	f := reg[otus.ComponentTypeSource]["file"]
	if f == nil {
		t.Error("file source factory not registered")
	}
	s := factory.GetSource(cfg)
	if s == nil {
		t.Error("failed to get source")
	}
}
