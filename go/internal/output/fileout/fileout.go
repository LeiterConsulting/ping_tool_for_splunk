package fileout

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
)

type Writer struct {
	path  string
	maxMB int
	file  *os.File
	buf   *bufio.Writer
}

func New(path string, maxMB int) (*Writer, error) {
	w := &Writer{path: path, maxMB: maxMB}
	if err := w.rotateIfNeeded(); err != nil {
		return nil, err
	}
	if err := w.openAppend(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Writer) rotateIfNeeded() error {
	if w.maxMB <= 0 {
		return nil
	}
	st, err := os.Stat(w.path)
	if err != nil {
		return nil // doesn't exist
	}
	if st.Size() < int64(w.maxMB)*1024*1024 {
		return nil
	}
	stamp := time.Now().UTC().Format("20060102_150405")
	arch := w.path
	if filepath.Ext(arch) == ".log" {
		arch = arch[:len(arch)-4]
	}
	arch = fmt.Sprintf("%s_%s.log", arch, stamp)
	if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
		return err
	}
	return os.Rename(w.path, arch)
}

func (w *Writer) openAppend() error {
	if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w.file = f
	w.buf = bufio.NewWriterSize(f, 256*1024)
	return nil
}

func (w *Writer) WriteOne(v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.buf.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

func (w *Writer) WritePingEvents(events []models.PingEvent) error {
	for i := range events {
		if err := w.WriteOne(events[i]); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) Flush() error {
	if w.buf != nil {
		return w.buf.Flush()
	}
	return nil
}

func (w *Writer) Close() error {
	if w.buf != nil {
		_ = w.buf.Flush()
	}
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

var _ io.Writer
