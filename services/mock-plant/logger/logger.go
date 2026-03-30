package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type FileLogger struct {
	file *os.File
	mu   sync.Mutex
}

func NewFileLogger(path string) (*FileLogger, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return &FileLogger{file: f}, nil
}

func (l *FileLogger) Write(data any) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = l.file.Write(append(bytes, '\n'))
	return err
}

func (l *FileLogger) Close() error {
	return l.file.Close()
}
