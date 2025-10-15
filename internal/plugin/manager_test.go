package plugin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewManager(t *testing.T) {}

func TestMananager_Initialize_Success(t *testing.T) {}

func TestManager_Initialize_DependencyOrder(t *testing.T) {}

func TestManager_Initialize_PluginError(t *testing.T) {}

func TestManager_Initialize_Timeout(t *testing.T) {}

func TestManager_Start_Success(t *testing.T) {}

func TestManager_Start_Error(t *testing.T) {}

func TestManager_Stop_Success(t *testing.T) {}

func TestManager_Stop_ContinueOnError(t *testing.T) {}

func TestManager_HealthCheck(t *testing.T) {}

func TestManager_HealthCheck_UnhealthyPlugin(t *testing.T) {}

func TestManager_GetStatus(t *testing.T) {}

func TestManager_GetStatus_NotFound(t *testing.T) {}

func TestManager_GetAllStatuses(t *testing.T) {
	r := NewRegistry()

	p1 := NewMockPlugin("plugin1", "gatherer", nil)
	p2 := NewMockPlugin("plugin2", "processor", nil)

	_ = r.Register(p1)
	_ = r.Register(p2)

	config := ManagerConfig{
		InitTimeout:  5 * time.Second,
		StartTimeout: 5 * time.Second,
		StopTimeout:  5 * time.Second,
	}

	manager := NewManager(config, r)
	_ = manager.Initialize(make(map[string]map[string]interface{}))

	statuses := manager.GetAllStatuses()
	assert.Len(t, statuses, 2)
	assert.Contains(t, statuses, "plugin1")
	assert.Contains(t, statuses, "plugin2")
}
