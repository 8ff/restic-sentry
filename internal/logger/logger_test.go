package logger

import (
	"testing"
)

func TestLogDoesNotPanic(t *testing.T) {
	log := New()
	// Should not panic on any level
	log.Info("hello world", map[string]any{"key": "value"})
	log.Warn("be careful")
	log.Error("something broke", map[string]any{"code": 42})
}
