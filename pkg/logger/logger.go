package logger

import (
	"fmt"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
)

type Config struct {
	AppName    string
	Level      string
	FileLevel  string
	MaxSize    int
	MaxBackups int
	MaxAge     int
	Port       int
	Version    string
	Fluentd    bool
	FluentHost string
}

func DefaultConfig() *Config {
	return &Config{
		AppName:    "ofr",
		Level:      "info",
		FileLevel:  "trace",
		MaxSize:    100,
		MaxBackups: 10,
		MaxAge:     10,
		Port:       0000,
		Version:    "v0.0.0",
	}
}

// New creates an instance of a logger hidden behind Logger interface
// today the implementation of a logger is logrus, but this may change in future
// as well as Logger interface itself to include With, WithFields, WithContext etc. methods
//
// https://peter.bourgon.org/go-best-practices-2016/#top-tip-9
// https://dave.cheney.net/2017/01/26/context-is-for-cancelation
func New(cfg *Config) (Logger, func(), error) {
	consoleLevel, err := log.ParseLevel(cfg.Level)
	if err != nil {
		log.Fatal("invalid log level: ", err)
	}

	fileLevel, err := log.ParseLevel(cfg.FileLevel)
	if err != nil {
		log.Fatal("invalid file log level: ", err)
	}

	var fluentDConfig *log.FluentDConfig
	if cfg.Fluentd {
		fluentDConfig = &log.FluentDConfig{
			FluentDHost: cfg.FluentHost,
			Level:       log.InfoLevel,
		}
	}

	closeLogger, err := log.Init(&log.Config{
		AppName:      cfg.AppName,
		FileName:     fmt.Sprintf("logs/%v-%v.log", cfg.AppName, cfg.Port),
		FileLevel:    fileLevel,
		ConsoleLevel: consoleLevel,
		MaxSize:      cfg.MaxSize,
		MaxBackups:   cfg.MaxBackups,
		MaxAge:       cfg.MaxAge,
	}, fluentDConfig, cfg.Version)

	if err != nil {
		return nil, nil, err
	}

	return &logrusLogger{}, closeLogger, nil
}

type Logger interface {
	Trace(string)
	Tracef(string, ...interface{})
	Debug(string)
	Debugf(string, ...interface{})
	Info(string)
	Infof(string, ...interface{})
	Warn(string)
	Warnf(string, ...interface{})
	Error(string)
	Errorf(string, ...interface{})
}

type logrusLogger struct{}

func (l *logrusLogger) Trace(s string)                    { log.Trace(s) }
func (l *logrusLogger) Tracef(s string, a ...interface{}) { log.Tracef(s, a...) }
func (l *logrusLogger) Debug(s string)                    { log.Debug(s) }
func (l *logrusLogger) Debugf(s string, a ...interface{}) { log.Debugf(s, a...) }
func (l *logrusLogger) Info(s string)                     { log.Infof(s) }
func (l *logrusLogger) Infof(s string, a ...interface{})  { log.Infof(s, a...) }
func (l *logrusLogger) Warn(s string)                     { log.Warnf(s) }
func (l *logrusLogger) Warnf(s string, a ...interface{})  { log.Warnf(s, a...) }
func (l *logrusLogger) Error(s string)                    { log.Error(s) }
func (l *logrusLogger) Errorf(s string, a ...interface{}) { log.Errorf(s, a...) }

type noopLogger struct{}

func (l *noopLogger) Trace(string)                  {}
func (l *noopLogger) Tracef(string, ...interface{}) {}
func (l *noopLogger) Debug(string)                  {}
func (l *noopLogger) Debugf(string, ...interface{}) {}
func (l *noopLogger) Info(string)                   {}
func (l *noopLogger) Infof(string, ...interface{})  {}
func (l *noopLogger) Warn(string)                   {}
func (l *noopLogger) Warnf(string, ...interface{})  {}
func (l *noopLogger) Error(string)                  {}
func (l *noopLogger) Errorf(string, ...interface{}) {}

// NewNoopLogger returns a logger that does nothing
func NewNoopLogger() Logger {
	return &noopLogger{}
}
