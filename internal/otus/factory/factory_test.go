package factory_test

import (
	"testing"

	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/factory"
	_ "firestige.xyz/otus/internal/sink/console"
	_ "firestige.xyz/otus/internal/source/afpacket"
	"firestige.xyz/otus/internal/source/file"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryInit(t *testing.T) {
	cfg := &file.FileCfg{}
	cfg.Name = "file"
	cfg.FilePath = "testdata/sample.pcap"

	reg := factory.GetRegistry()
	assert.NotEmpty(t, reg, "registry should not be empty")

	sourceFactories := reg[otus.ComponentTypeSource]
	require.NotNil(t, sourceFactories, "source factories should not be nil")

	f := sourceFactories[file.Name]
	assert.NotNil(t, f, "file source factory should be registered")

	source := factory.GetSource(cfg)
	assert.NotNil(t, source, "GetSource should return a source instance")
}
