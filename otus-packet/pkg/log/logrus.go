package log

import "github.com/sirupsen/logrus"

// TODO log的entry是什么东西？怎么用的，在这里有什么影响？

var logger *logrusLogger

type logrusLogger struct {
	log *logrus.Logger
}

func init() {
	logger = &logrusLogger{
		log: logrus.New(),
	}
}

func GetLogger() Logger {
	return logger
}

func (l *logrusLogger) Print(args ...interface{}) {
	l.log.Print(args...)
}

func (l *logrusLogger) Printf(format string, args ...interface{}) {
	l.log.Printf(format, args...)
}

func (l *logrusLogger) Trace(args ...interface{}) {
	l.log.Trace(args...)
}

func (l *logrusLogger) Tracef(format string, args ...interface{}) {
	l.log.Tracef(format, args...)
}

func (l *logrusLogger) Debug(args ...interface{}) {
	l.log.Debug(args...)
}

func (l *logrusLogger) Debugf(format string, args ...interface{}) {
	l.log.Debugf(format, args...)
}

func (l *logrusLogger) Info(args ...interface{}) {
	l.log.Info(args...)
}

func (l *logrusLogger) Infof(format string, args ...interface{}) {
	l.log.Infof(format, args...)
}

func (l *logrusLogger) Warn(args ...interface{}) {
	l.log.Warn(args...)
}

func (l *logrusLogger) Warnf(format string, args ...interface{}) {
	l.log.Warnf(format, args...)
}

func (l *logrusLogger) Error(args ...interface{}) {
	l.log.Error(args...)
}

func (l *logrusLogger) Errorf(format string, args ...interface{}) {
	l.log.Errorf(format, args...)
}

func (l *logrusLogger) Fatal(args ...interface{}) {
	l.log.Fatal(args...)
}

func (l *logrusLogger) Fatalf(format string, args ...interface{}) {
	l.log.Fatalf(format, args...)
}

func (l *logrusLogger) Panic(args ...interface{}) {
	l.log.Panic(args...)
}

func (l *logrusLogger) Panicf(format string, args ...interface{}) {
	l.log.Panicf(format, args...)
}

func (l *logrusLogger) WithField(field string, value interface{}) Logger {
	l.log.WithField(field, value)
	return l
}

func (l *logrusLogger) WithFields(fields map[string]interface{}) Logger {
	l.log.WithFields(logrus.Fields(fields))
	return l
}

func (l *logrusLogger) WithError(err error) Logger {
	l.log.WithError(err)
	return l
}

func (l *logrusLogger) IsTraceEnabled() bool {
	return l.log.IsLevelEnabled(logrus.TraceLevel)
}

func (l *logrusLogger) IsDebugEnabled() bool {
	return l.log.IsLevelEnabled(logrus.DebugLevel)
}

func (l *logrusLogger) IsInfoEnabled() bool {
	return l.log.IsLevelEnabled(logrus.InfoLevel)
}
