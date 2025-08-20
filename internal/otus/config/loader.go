package config

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/internal/plugin"
	"github.com/spf13/viper"
)

func Load(path string) (*OtusConfig, error) {
	v := viper.New()

	// 设置配置文件路径和名称
	dir := filepath.Dir(path)
	filename := filepath.Base(path)
	fileExt := filepath.Ext(filename)
	nameWithoutExt := strings.TrimSuffix(filename, fileExt)

	v.SetConfigName(nameWithoutExt)
	v.SetConfigType(strings.TrimPrefix(fileExt, "."))
	v.AddConfigPath(dir)

	v.SetEnvPrefix("OTUS")
	v.AutomaticEnv()                                             // 自动读取环境变量
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_")) // 替换环境变量中的点和连字符

	// 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var config *OtusConfig

	// 使用 viper 的 Unmarshal，自动处理 mapstructure
	if err := v.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 应用默认配置
	applyDefaults(config)

	propagateCommonFieldsInPipes(config.Pipes)
	return config, nil
}

// applyDefaults 应用默认配置
func applyDefaults(otusConfig *OtusConfig) {
	// 确保 Logger 配置不为空，如果为空则提供默认值
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
}

// propagateCommonFieldsInPipes propagates the common fields to every module and the dependency plugin.
func propagateCommonFieldsInPipes(pipes []*PipeConfig) {
	for _, pipe := range pipes {
		pipe.Capture.CommonFields = pipe.CommonConfig
		pipe.Processor.CommonFields = pipe.CommonConfig
		pipe.Sender.CommonFields = pipe.CommonConfig
		propagateCommonFieldsInStruct(pipe.Capture, pipe.CommonConfig)
		propagateCommonFieldsInStruct(pipe.Processor, pipe.CommonConfig)
		propagateCommonFieldsInStruct(pipe.Sender, pipe.CommonConfig)
	}
}

// propagate the common fields to the fields that is one of `plugin.config` or `[]plugin.config` types.
func propagateCommonFieldsInStruct(cfg interface{}, commonFields *config.CommonFields) {
	v := reflect.ValueOf(cfg)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	for i := 0; i < v.Type().NumField(); i++ {
		fieldVal := v.Field(i).Interface()
		if arr, ok := fieldVal.([]plugin.Config); arr != nil && ok {
			for _, pc := range arr {
				propagateCommonFields(pc, commonFields)
			}
		} else if pc, ok := fieldVal.(plugin.Config); pc != nil && ok {
			propagateCommonFields(pc, commonFields)
		}
	}
}

func propagateCommonFields(pc plugin.Config, cf *config.CommonFields) {
	v := reflect.ValueOf(cf)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		if tagVal := t.Field(i).Tag.Get(config.TagName); tagVal != "" {
			pc[strings.ToLower(config.CommonFieldsName)+"_"+tagVal] = v.Field(i).Interface()
		}
	}
}
