package log

import "gopkg.in/natefinch/lumberjack.v2"

type FileAppenderOpt struct {
	Filename   string `mapstructure:"filename"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
	Compress   bool   `mapstructure:"compress"`
}

func (m *MultiWriter) AddFileAppender(options FileAppenderOpt) *MultiWriter {
	writer := &lumberjack.Logger{
		Filename:   options.Filename,
		MaxSize:    options.MaxSize,    // megabytes
		MaxBackups: options.MaxBackups, // number of backups
		MaxAge:     options.MaxAge,     // days
		Compress:   options.Compress,   // compress the backups
	}
	m.writers = append(m.writers, writer)
	return m
}
