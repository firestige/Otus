package log

import (
	"fmt"
	"strings"
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

type Fields map[string]interface{}

func (f Fields) String() string {
	str := make([]string, 0)
	for k, v := range f {
		str = append(str, fmt.Sprintf("%s=%+v", k, v))
	}
	return strings.Join(str, " ")
}

func (f Fields) WithFields(newFields Fields) Fields {
	allFields := make(Fields)

	for k, v := range f {
		allFields[k] = v
	}

	for k, v := range newFields {
		allFields[k] = v
	}

	return allFields
}
