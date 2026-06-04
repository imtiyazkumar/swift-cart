package logger

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestParseLevelDefaultsToInfo(t *testing.T) {
	if got := parseLevel("unknown"); got != zapcore.InfoLevel {
		t.Fatalf("expected info level, got %v", got)
	}
}

func TestNewCreatesLogger(t *testing.T) {
	log := New("debug")
	if log == nil {
		t.Fatal("expected logger")
	}
	_ = log.Sync()
}
