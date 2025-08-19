package log

import "time"

type LoggerConfig struct {
	Level     string           `yaml:"level"`
	Pattern   string           `yaml:"pattern"`
	Time      string           `yaml:"time"`
	Appenders []AppenderConfig `yaml:"appenders"`
	Formatter *FormatterConfig `yaml:"formatter,omitempty"`

	// 新增配置
	BufferSize    int           `yaml:"buffer_size,omitempty"`
	FlushInterval time.Duration `yaml:"flush_interval,omitempty"`
}

type AppenderConfig struct {
	Type    string                 `yaml:"type"`
	Level   string                 `yaml:"level,omitempty"`
	Options map[string]interface{} `yaml:"options,omitempty"`
}

type FormatterConfig struct {
	EnableColors   bool `yaml:"enable_colors,omitempty"`
	FullTimestamp  bool `yaml:"full_timestamp,omitempty"`
	DisableSorting bool `yaml:"disable_sorting,omitempty"`
}

type FileAppenderOptions struct {
	Filename   string `yaml:"filename"`
	MaxSize    int    `yaml:"maxsize,omitempty"` // MB
	MaxAge     int    `yaml:"maxage,omitempty"`  // days
	MaxBackups int    `yaml:"maxbackups,omitempty"`
	Compress   bool   `yaml:"compress,omitempty"`
}
