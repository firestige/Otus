package log

import (
	"sync"
)

type Logger interface {
	Print(args ...interface{})
	Printf(format string, args ...interface{})

	Trace(args ...interface{})
	Tracef(format string, args ...interface{})

	Debug(args ...interface{})
	Debugf(format string, args ...interface{})

	Info(args ...interface{})
	Infof(format string, args ...interface{})

	Warn(args ...interface{})
	Warnf(format string, args ...interface{})

	Error(args ...interface{})
	Errorf(format string, args ...interface{})

	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})

	Panic(args ...interface{})
	Panicf(format string, args ...interface{})

	WithField(field string, value interface{}) Logger
	WithFields(fields map[string]interface{}) Logger
	WithError(err error) Logger

	IsTraceEnabled() bool
	IsDebugEnabled() bool
	IsInfoEnabled() bool
}

var (
	once   sync.Once
	logger Logger
)

func GetLogger() Logger {
	return logger
}

func Init(cfg *LoggerConfig) {
	once.Do(func() {
		var err error
		err = initByConfig(cfg)
		if err != nil {
			panic(err)
		}
	})
}
