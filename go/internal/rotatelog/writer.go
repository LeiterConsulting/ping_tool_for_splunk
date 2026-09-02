package rotatelog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Writer struct {
	mu        sync.Mutex
	path      string
	maxBytes  int64
	retention int
	file      *os.File
	size      int64
}

func New(path string, maxBytes int64, retention int) (*Writer, error) {
	if path == "" {
		return nil, fmt.Errorf("rotating log path must not be empty")
	}
	if maxBytes < 1 {
		return nil, fmt.Errorf("rotating log maximum must be positive")
	}
	if retention < 1 {
		return nil, fmt.Errorf("rotating log retention must be positive")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	w := &Writer{path: filepath.Clean(path), maxBytes: maxBytes, retention: retention}
	if err := w.open(); err != nil {
		return nil, err
	}
	if w.size >= w.maxBytes {
		if err := w.rotate(); err != nil {
			w.file.Close()
			return nil, err
		}
	}
	return w, nil
}

func (w *Writer) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if w.size > 0 && w.size+int64(len(data)) > w.maxBytes {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	written, err := w.file.Write(data)
	w.size += int64(written)
	return written, err
}

func (w *Writer) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return os.ErrClosed
	}
	return w.file.Sync()
}

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *Writer) open() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	w.file = file
	w.size = info.Size()
	return nil
}

func (w *Writer) rotate() error {
	if w.file != nil {
		if err := w.file.Sync(); err != nil {
			return err
		}
		if err := w.file.Close(); err != nil {
			return err
		}
		w.file = nil
	}
	oldest := fmt.Sprintf("%s.%d", w.path, w.retention)
	if err := os.Remove(oldest); err != nil && !os.IsNotExist(err) {
		return err
	}
	for index := w.retention - 1; index >= 1; index-- {
		from := fmt.Sprintf("%s.%d", w.path, index)
		to := fmt.Sprintf("%s.%d", w.path, index+1)
		if err := os.Rename(from, to); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Rename(w.path, w.path+".1"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return w.open()
}
