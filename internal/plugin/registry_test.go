package plugin

import (
	"fmt"
	"testing"

	"firestige.xyz/otus/pkg/plugin"
	"github.com/stretchr/testify/assert"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()

	assert.NotNil(t, r)
	assert.NotNil(t, r.plugins)
	assert.NotNil(t, r.types)

	assert.NotNil(t, plugin.Get)
}

func TestRegistry_Register_Success(t *testing.T) {
	r := NewRegistry()

	p := NewMockPlugin("test-plugin", "gatherer", nil)

	err := r.Register(p)
	assert.NoError(t, err)

	actual, err := r.Get("test-plugin")
	assert.NoError(t, err)
	assert.NotNil(t, actual)
	assert.Equal(t, "test-plugin", actual.Metadata().Name)
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	r := NewRegistry()

	p1 := NewMockPlugin("test-plugin", "gatherer", nil)
	p2 := NewMockPlugin("test-plugin", "forwarder", nil)

	err1 := r.Register(p1)
	err2 := r.Register(p2)

	assert.NoError(t, err1)
	assert.Error(t, err2)
	assert.Contains(t, err2.Error(), "already registered")
}

func TestRegistry_Register_InvalidType(t *testing.T) {
	r := NewRegistry()
	p := NewMockPlugin("test-plugin", "invalid-type", nil)

	err := r.Register(p)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid plugin type")
}

func TestRegistry_Get_Success(t *testing.T) {
	r := NewRegistry()
	p := NewMockPlugin("test-plugin", "gatherer", nil)

	_ = r.Register(p)

	retrieved, err := r.Get("test-plugin")
	assert.NoError(t, err)
	assert.Equal(t, p, retrieved)
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()

	_, err := r.Get("non-existent-plugin")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRegistry_List_Success(t *testing.T) {
	r := NewRegistry()
	p1 := NewMockPlugin("plugin-one", "gatherer", nil)
	p2 := NewMockPlugin("plugin-two", "forwarder", nil)
	p3 := NewMockPlugin("plugin-three", "gatherer", nil)

	_ = r.Register(p1)
	_ = r.Register(p2)
	_ = r.Register(p3)

	gatherers := r.List("gatherer")

	assert.Len(t, gatherers, 2)
	assert.Contains(t, gatherers, p1)
	assert.Contains(t, gatherers, p3)

	forwarders := r.List("forwarder")
	assert.Len(t, forwarders, 1)
	assert.Contains(t, forwarders, p2)

	processors := r.List("processor")
	assert.Len(t, processors, 0)
}

func TestRegistry_GetLoadOrder_NoDependency(t *testing.T) {
	r := NewRegistry()
	p1 := NewMockPlugin("plugin-one", "gatherer", nil)
	p2 := NewMockPlugin("plugin-two", "processor", nil)
	p3 := NewMockPlugin("plugin-three", "forwarder", nil)

	_ = r.Register(p1)
	_ = r.Register(p2)
	_ = r.Register(p3)

	loadOrder, err := r.GetLoadOrder()
	assert.NoError(t, err)
	assert.Len(t, loadOrder, 3)
	assert.Contains(t, loadOrder, "plugin-one")
	assert.Contains(t, loadOrder, "plugin-two")
	assert.Contains(t, loadOrder, "plugin-three")
}

func TestRegistry_GetLoadOrder_WithDependency(t *testing.T) {
	r := NewRegistry()
	pA := NewMockPlugin("plugin-A", "gatherer", nil)
	pB := NewMockPlugin("plugin-B", "processor", []string{"plugin-A"})
	pC := NewMockPlugin("plugin-C", "forwarder", []string{"plugin-B"})

	_ = r.Register(pA)
	_ = r.Register(pB)
	_ = r.Register(pC)

	loadOrder, err := r.GetLoadOrder()
	assert.NoError(t, err)
	assert.Len(t, loadOrder, 3)
	assert.Equal(t, "plugin-A", loadOrder[0])
	assert.Equal(t, "plugin-B", loadOrder[1])
	assert.Equal(t, "plugin-C", loadOrder[2])
}

func TestRegistry_GetLoadOrder_ComplexDependency(t *testing.T) {
	r := NewRegistry()

	// complex dependency graph:
	//   D -> B -> A
	//   D -> C -> A
	pA := NewMockPlugin("plugin-A", "gatherer", nil)
	pB := NewMockPlugin("plugin-B", "processor", []string{"plugin-A"})
	pC := NewMockPlugin("plugin-C", "forwarder", []string{"plugin-A"})
	pD := NewMockPlugin("plugin-D", "gatherer", []string{"plugin-B", "plugin-C"})

	_ = r.Register(pA)
	_ = r.Register(pB)
	_ = r.Register(pC)
	_ = r.Register(pD)

	loadOrder, err := r.GetLoadOrder()
	assert.NoError(t, err)
	assert.Len(t, loadOrder, 4)

	// A must come before B and C, which must come before D
	assert.Equal(t, "plugin-A", loadOrder[0])
	assert.Equal(t, "plugin-B", loadOrder[1])
	assert.Equal(t, "plugin-C", loadOrder[2])
	assert.Equal(t, "plugin-D", loadOrder[3])
}

func TestRegistry_GetLoadOrder_CircularDependency(t *testing.T) {
	r := NewRegistry()

	// circular dependency: A -> B -> C -> A
	pA := NewMockPlugin("plugin-A", "gatherer", []string{"plugin-B"})
	pB := NewMockPlugin("plugin-B", "processor", []string{"plugin-C"})
	pC := NewMockPlugin("plugin-C", "forwarder", []string{"plugin-A"})

	_ = r.Register(pA)
	_ = r.Register(pB)
	_ = r.Register(pC)

	_, err := r.GetLoadOrder()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestRegistry_GetLoadOrder_MissingDependency(t *testing.T) {
	r := NewRegistry()
	p := NewMockPlugin("plugin-with-missing-dep", "gatherer", []string{"missing-dep"})

	_ = r.Register(p)

	_, err := r.GetLoadOrder()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown dependency")
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	const numGoroutines = 100
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			p := NewMockPlugin(fmt.Sprintf("plugin-%d", id), "gatherer", nil)
			_ = r.Register(p)
			done <- true
		}(i)
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	gatherers := r.List("gatherer")
	assert.Len(t, gatherers, numGoroutines)
}

func TestRegistry_GlobalRegistry(t *testing.T) {
	p := NewMockPlugin("global-plugin", "gatherer", nil)

	err := plugin.Register(p)
	assert.NoError(t, err)

	retrieved, err := plugin.Get("global-plugin")
	assert.NoError(t, err)
	assert.Equal(t, p, retrieved)

	list := plugin.List("gatherer")
	assert.Len(t, list, 1)
	assert.Contains(t, list, p)
}
