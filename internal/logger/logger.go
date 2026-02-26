package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

type Level string

const (
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

type Entry struct {
	Time    string `json:"time"`
	Level   Level  `json:"level"`
	Message string `json:"msg"`
	Fields  map[string]any `json:"fields,omitempty"`
}

type Logger struct {
	mu sync.Mutex
}

func New() *Logger {
	return &Logger{}
}

func (l *Logger) log(level Level, msg string, fields map[string]any) {
	entry := Entry{
		Time:    time.Now().UTC().Format(time.RFC3339),
		Level:   level,
		Message: msg,
		Fields:  fields,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal log entry: %v\n", err)
		return
	}
	data = append(data, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	os.Stderr.Write(data)
}

func (l *Logger) Info(msg string, fields ...map[string]any) {
	var f map[string]any
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelInfo, msg, f)
}

func (l *Logger) Warn(msg string, fields ...map[string]any) {
	var f map[string]any
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelWarn, msg, f)
}

func (l *Logger) Error(msg string, fields ...map[string]any) {
	var f map[string]any
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelError, msg, f)
}
