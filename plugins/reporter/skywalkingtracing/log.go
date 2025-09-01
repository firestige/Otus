package skywalkingtracing

import (
	gosiplog "github.com/ghettovoice/gosip/log"
	"github.com/sirupsen/logrus"
)

// LoggerAdapter 适配器，将 satellite 的 Logger 适配为 gosip 的 Logger 接口
type LoggerAdapter struct {
	logger *logrus.Entry
}

func (la *LoggerAdapter) Fields() gosiplog.Fields {
	return gosiplog.Fields{}
}

func (la *LoggerAdapter) WithFields(fields map[string]interface{}) gosiplog.Logger {
	la.logger.WithFields(fields)
	return la
}

// 添加缺失的 Prefix 方法
func (la *LoggerAdapter) Prefix() string {
	return ""
}

func (la *LoggerAdapter) WithPrefix(prefix string) gosiplog.Logger {
	// 可以选择不实现或者存储前缀
	return la
}

func (la *LoggerAdapter) Print(args ...interface{}) {
	la.logger.Print(args...)
}

func (la *LoggerAdapter) Printf(format string, args ...interface{}) {
	la.logger.Printf(format, args...)
}

func (la *LoggerAdapter) Trace(args ...interface{}) {
	la.logger.Trace(args...)
}

func (la *LoggerAdapter) Tracef(format string, args ...interface{}) {
	la.logger.Tracef(format, args...)
}

func (la *LoggerAdapter) Debug(args ...interface{}) {
	la.logger.Debug(args...)
}

func (la *LoggerAdapter) Debugf(format string, args ...interface{}) {
	la.logger.Debugf(format, args...)
}

func (la *LoggerAdapter) Info(args ...interface{}) {
	la.logger.Info(args...)
}

func (la *LoggerAdapter) Infof(format string, args ...interface{}) {
	la.logger.Infof(format, args...)
}

func (la *LoggerAdapter) Warn(args ...interface{}) {
	la.logger.Warn(args...)
}

func (la *LoggerAdapter) Warnf(format string, args ...interface{}) {
	la.logger.Warnf(format, args...)
}

func (la *LoggerAdapter) Error(args ...interface{}) {
	la.logger.Error(args...)
}

func (la *LoggerAdapter) Errorf(format string, args ...interface{}) {
	la.logger.Errorf(format, args...)
}

func (la *LoggerAdapter) Fatal(args ...interface{}) {
	la.logger.Fatal(args...)
}

func (la *LoggerAdapter) Fatalf(format string, args ...interface{}) {
	la.logger.Fatalf(format, args...)
}

func (la *LoggerAdapter) Panic(args ...interface{}) {
	la.logger.Panic(args...)
}

func (la *LoggerAdapter) Panicf(format string, args ...interface{}) {
	la.logger.Panicf(format, args...)
}

func (la *LoggerAdapter) SetLevel(level uint32) {
	la.SetLevel(level)
}
