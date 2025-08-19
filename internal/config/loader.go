package config

import (
	"fmt"
	"os"

	"firestige.xyz/otus/internal/log"
	"gopkg.in/yaml.v3"
)

func Load(path string) (*OtusConfig, error) {
	var config OtusConfig
	err := loadConfigFile(path, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func loadConfigFile(path string, otusConfig *OtusConfig) error {
	// 检查文件是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist: %s", path)
	}

	// 读取配置文件内容
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	// 解析 YAML 内容到配置结构体
	if err := yaml.Unmarshal(data, otusConfig); err != nil {
		return fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	// 确保 Logger 配置不为空，如果为空则提供默认值 - 仅以 info 级别输出 console 日志
	if otusConfig.Logger == nil {
		otusConfig.Logger = &log.LoggerConfig{
			Level:   "info",
			Pattern: "%time [%level] %caller: %msg%n",
			Time:    "2006-01-02 15:04:05",
			Appenders: []log.AppenderConfig{
				{
					Type:  "console",
					Level: "info",
				},
			},
		}
	}

	return nil
}
