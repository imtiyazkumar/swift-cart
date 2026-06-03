package logger

import (
    "github.com/labstack/echo/v4"
    "github.com/labstack/gommon/log"
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
    "io"
    "os"
)

func New(level string) *zap.Logger {
    cfg := zap.NewProductionConfig()
    cfg.Encoding = "json"
    cfg.EncoderConfig.TimeKey = "timestamp"
    cfg.EncoderConfig.LevelKey = "level"
    cfg.EncoderConfig.MessageKey = "msg"
    cfg.EncoderConfig.CallerKey = "caller"
    cfg.EncoderConfig.StacktraceKey = "stacktrace"
    cfg.Level = zap.NewAtomicLevelAt(parseLevel(level))
    logger, err := cfg.Build()
    if err != nil {
        panic(err)
    }
    return logger
}

func parseLevel(lvl string) zapcore.Level {
    switch lvl {
    case "debug":
        return zapcore.DebugLevel
    case "info":
        return zapcore.InfoLevel
    case "warn":
        return zapcore.WarnLevel
    case "error":
        return zapcore.ErrorLevel
    default:
        return zapcore.InfoLevel
    }
}

// EchoZapAdapter adapts zap.SugaredLogger to echo.Logger interface.
type EchoZapAdapter struct {
    sugared *zap.SugaredLogger
    out     io.Writer
}

func (e *EchoZapAdapter) Output() io.Writer { return e.out }
func (e *EchoZapAdapter) SetOutput(w io.Writer) { e.out = w }
func (e *EchoZapAdapter) Prefix() string { return "" }
func (e *EchoZapAdapter) SetPrefix(p string) {}
func (e *EchoZapAdapter) Level() log.Lvl { return log.INFO }
func (e *EchoZapAdapter) SetLevel(v log.Lvl) {}
func (e *EchoZapAdapter) SetHeader(h string) {}

func NewEchoLogger(z *zap.Logger) echo.Logger {
    return &EchoZapAdapter{sugared: z.Sugar(), out: os.Stdout}
}

// Echo Logger methods implementation.
func (e *EchoZapAdapter) Debug(i ...interface{}) { e.sugared.Debug(i...) }
func (e *EchoZapAdapter) Debugf(format string, args ...interface{}) { e.sugared.Debugf(format, args...) }
func (e *EchoZapAdapter) Debugj(j log.JSON) { e.sugared.Debugw("", "json", j) }
func (e *EchoZapAdapter) Info(i ...interface{}) { e.sugared.Info(i...) }
func (e *EchoZapAdapter) Infof(format string, args ...interface{}) { e.sugared.Infof(format, args...) }
func (e *EchoZapAdapter) Infoj(j log.JSON) { e.sugared.Infow("", "json", j) }
func (e *EchoZapAdapter) Warn(i ...interface{}) { e.sugared.Warn(i...) }
func (e *EchoZapAdapter) Warnf(format string, args ...interface{}) { e.sugared.Warnf(format, args...) }
func (e *EchoZapAdapter) Warnj(j log.JSON) { e.sugared.Warnw("", "json", j) }
func (e *EchoZapAdapter) Error(i ...interface{}) { e.sugared.Error(i...) }
func (e *EchoZapAdapter) Errorf(format string, args ...interface{}) { e.sugared.Errorf(format, args...) }
func (e *EchoZapAdapter) Errorj(j log.JSON) { e.sugared.Errorw("", "json", j) }
func (e *EchoZapAdapter) Fatal(i ...interface{}) { e.sugared.Fatal(i...) }
func (e *EchoZapAdapter) Fatalf(format string, args ...interface{}) { e.sugared.Fatalf(format, args...) }
func (e *EchoZapAdapter) Fatalj(j log.JSON) { e.sugared.Fatalw("", "json", j) }

func (e *EchoZapAdapter) Print(i ...interface{}) { e.sugared.Info(i...) }
func (e *EchoZapAdapter) Printf(format string, args ...interface{}) { e.sugared.Infof(format, args...) }
func (e *EchoZapAdapter) Printj(j log.JSON) { e.sugared.Infow("", "json", j) }

// Panic methods to satisfy echo.Logger interface.
func (e *EchoZapAdapter) Panic(i ...interface{}) { e.sugared.Panic(i...) }
func (e *EchoZapAdapter) Panicf(format string, args ...interface{}) { e.sugared.Panicf(format, args...) }
func (e *EchoZapAdapter) Panicj(j log.JSON) { e.sugared.Panicw("", "json", j) }

