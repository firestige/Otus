package capture_test

import (
	"os"
	"path/filepath"
	"testing"

	"firestige.xyz/otus/internal/otus/config"
	"firestige.xyz/otus/internal/otus/module/capture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestCaptureConfigFromRootConfigYml(t *testing.T) {
	// 获取项目根目录
	workspaceRoot := "/workspaces/Otus"
	configPath := filepath.Join(workspaceRoot, "config.yml")

	// 检查根目录的 config.yml 是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("Root config.yml not found, skipping test")
	}

	// 读取并解析 config.yml
	data, err := os.ReadFile(configPath)
	require.NoError(t, err, "Failed to read config.yml")

	// 解析为通用的 map 结构
	var rawConfig map[string]interface{}
	err = yaml.Unmarshal(data, &rawConfig)
	require.NoError(t, err, "Failed to unmarshal config.yml")

	t.Logf("Root config.yml structure: %+v", rawConfig)

	// 检查是否包含 capture 相关配置
	if global, exists := rawConfig["global"]; !exists {
		t.Skip("No global section found in root config.yml")
		if _, exists := global.(map[string]interface{})["capture"]; !exists {
			t.Skip("No capture section found in root config.yml")
		}
	}

	// 使用项目的配置加载器加载配置
	otusConfig, err := config.Load(configPath)
	require.NoError(t, err, "Failed to load config using config.Load()")

	t.Logf("Loaded OtusConfig: %+v", otusConfig)

	// 验证是否正确注入到 capture.Config
	// 注意：这里需要根据实际的 OtusConfig 结构进行调整
	if otusConfig.Global.Capture != nil {
		validateCaptureConfig(t, otusConfig.Global.Capture)
	} else {
		t.Log("No capture configuration found in loaded config")
	}
}

// validateCaptureConfig 验证 capture 配置的辅助函数
func validateCaptureConfig(t *testing.T, cfg *capture.Config) {
	assert.NotNil(t, cfg, "Capture config should not be nil")
	t.Log("validate capture config")
	t.Logf("%+v", cfg)
	if cfg.CommonFields != nil {
		t.Logf("PipeName: %s", cfg.CommonFields.PipeName)
	}

	if cfg.SnifferConfig != nil {
		t.Logf("SnifferConfig: %+v", cfg.SnifferConfig)
		// 这里可以添加具体的 sniffer 配置验证
	}

	if cfg.CodecConfig != nil {
		t.Logf("CodecConfig: %+v", cfg.CodecConfig)
		// 这里可以添加具体的 codec 配置验证
	}
}
