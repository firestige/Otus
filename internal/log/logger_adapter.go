package log

import (
	"os"

	"github.com/sirupsen/logrus"
)

type LoggerConfig struct {
	Pattern  string `mapstructure:"pattern"`
	Time     string `mapstructure:"time"`
	Level    string `mapstructure:"level"`
	Appender string `mapstructure:"appender"`
}

type logrusAdapter struct {
	entry *logrus.Entry
}

func initByConfig(cfg *LoggerConfig) error {
	l := logrus.New()
	l.SetFormatter(&formatter{
		pattern: cfg.Pattern,
		time:    cfg.Time,
	})
	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	l.SetLevel(level)

	// l.SetReportCaller(true)

	l.SetOutput(NewMultiWriter().Add(os.Stdout))

	logger = &logrusAdapter{
		entry: logrus.NewEntry(l),
	}
	return nil
}

func (l *logrusAdapter) Print(args ...interface{})                 { l.entry.Print(args...) }
func (l *logrusAdapter) Printf(format string, args ...interface{}) { l.entry.Printf(format, args...) }

func (l *logrusAdapter) Trace(args ...interface{})                 { l.entry.Trace(args...) }
func (l *logrusAdapter) Tracef(format string, args ...interface{}) { l.entry.Tracef(format, args...) }

func (l *logrusAdapter) Debug(args ...interface{})                 { l.entry.Debug(args...) }
func (l *logrusAdapter) Debugf(format string, args ...interface{}) { l.entry.Debugf(format, args...) }

func (l *logrusAdapter) Info(args ...interface{})                 { l.entry.Info(args...) }
func (l *logrusAdapter) Infof(format string, args ...interface{}) { l.entry.Infof(format, args...) }

func (l *logrusAdapter) Warn(args ...interface{})                 { l.entry.Warn(args...) }
func (l *logrusAdapter) Warnf(format string, args ...interface{}) { l.entry.Warnf(format, args...) }

func (l *logrusAdapter) Error(args ...interface{})                 { l.entry.Error(args...) }
func (l *logrusAdapter) Errorf(format string, args ...interface{}) { l.entry.Errorf(format, args...) }

func (l *logrusAdapter) Fatal(args ...interface{})                 { l.entry.Fatal(args...) }
func (l *logrusAdapter) Fatalf(format string, args ...interface{}) { l.entry.Fatalf(format, args...) }

func (l *logrusAdapter) Panic(args ...interface{})                 { l.entry.Panic(args...) }
func (l *logrusAdapter) Panicf(format string, args ...interface{}) { l.entry.Panicf(format, args...) }

func (l *logrusAdapter) WithField(field string, value interface{}) Logger {
	return &logrusAdapter{entry: l.entry.WithField(field, value)}
}
func (l *logrusAdapter) WithFields(fields map[string]interface{}) Logger {
	return &logrusAdapter{entry: l.entry.WithFields(fields)}
}
func (l *logrusAdapter) WithError(err error) Logger {
	return &logrusAdapter{entry: l.entry.WithError(err)}
}

func (l *logrusAdapter) IsTraceEnabled() bool {
	return l.entry.Logger.IsLevelEnabled(logrus.TraceLevel)
}
func (l *logrusAdapter) IsDebugEnabled() bool {
	return l.entry.Logger.IsLevelEnabled(logrus.DebugLevel)
}
func (l *logrusAdapter) IsInfoEnabled() bool {
	return l.entry.Logger.IsLevelEnabled(logrus.InfoLevel)
}
