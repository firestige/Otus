package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewLoader(t *testing.T) {
	r := NewRegistry()
	config := LoaderConfig{
		Mode:     StaticMode,
		Path:     "./testdata/plugins",
		Patterns: []string{"*.so"},
	}

	loader := NewLoader(config, r)

	assert.NotNil(t, loader)
	assert.Equal(t, config, loader.config)
	assert.Equal(t, r, loader.registry)
}

func TestLoader_Load_Static(t *testing.T) {
}

func TestLoader_Load_Static_Circular_Dependency(t *testing.T) {}

func TestLoader_Discover_Plugins(t *testing.T) {}

func TestLoader_Discover_PluginsNotFound(t *testing.T) {}

func TestLoader_Discover_InvalidDirectory(t *testing.T) {}

func TestLoader_LoadPlugin_FileNotFound(t *testing.T) {}
