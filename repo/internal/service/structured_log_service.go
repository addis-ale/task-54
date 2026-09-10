package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type StructuredLogService struct {
	path string
	mu   sync.Mutex
}

func NewStructuredLogService(path string) *StructuredLogService {
	return &StructuredLogService{path: path}
}

func (s *StructuredLogService) Path() string {
	return s.path
}

func (s *StructuredLogService) Log(level, event string, fields map[string]any) error {
	if s.path == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create structured log directory: %w", err)
	}

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open structured log file: %w", err)
	}
	defer f.Close()

	record := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"level":     level,
		"event":     event,
	}
	for k, v := range fields {
		record[k] = v
	}

	raw, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal structured log record: %w", err)
	}

	if _, err := f.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("write structured log record: %w", err)
	}

	return nil
}
