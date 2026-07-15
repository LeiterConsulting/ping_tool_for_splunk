package singleinstance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var ErrAlreadyRunning = errors.New("another ping monitor process already owns this deployment")

type Lock struct {
	file *os.File
	path string
}

type metadata struct {
	PID       int    `json:"pid"`
	Hostname  string `json:"hostname"`
	StartedAt string `json:"started_at"`
}

func Acquire(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}
	file, err := acquireFile(path)
	if err != nil {
		if errors.Is(err, ErrAlreadyRunning) {
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("acquire deployment lock: %w", err)
	}

	hostname, _ := os.Hostname()
	if err := file.Truncate(0); err != nil {
		_ = releaseFile(file)
		return nil, fmt.Errorf("truncate deployment lock: %w", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		_ = releaseFile(file)
		return nil, fmt.Errorf("seek deployment lock: %w", err)
	}
	if err := json.NewEncoder(file).Encode(metadata{
		PID:       os.Getpid(),
		Hostname:  hostname,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		_ = releaseFile(file)
		return nil, fmt.Errorf("write deployment lock: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = releaseFile(file)
		return nil, fmt.Errorf("sync deployment lock: %w", err)
	}
	return &Lock{file: file, path: path}, nil
}

func (l *Lock) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := releaseFile(l.file)
	l.file = nil
	return err
}
