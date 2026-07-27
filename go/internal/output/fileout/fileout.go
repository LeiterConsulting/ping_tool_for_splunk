package fileout

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/LeiterConsulting/ping_tool_for_splunk/go/internal/models"
)

type Options struct {
	Path            string
	MaxSizeMB       int
	RetentionFiles  int
	RetentionDays   int
	CompressRotated bool
}

type Status struct {
	Path                 string    `json:"path"`
	CurrentBytes         int64     `json:"current_bytes"`
	MaxBytes             int64     `json:"max_bytes"`
	RetentionFiles       int       `json:"retention_files"`
	RetentionDays        int       `json:"retention_days"`
	CompressRotated      bool      `json:"compress_rotated"`
	RotationCount        uint64    `json:"rotation_count"`
	LastRotationAt       time.Time `json:"last_rotation_at,omitempty"`
	LastArchivePath      string    `json:"last_archive_path,omitempty"`
	LastMaintenanceError string    `json:"last_maintenance_error,omitempty"`
}

type Writer struct {
	mu                   sync.Mutex
	opts                 Options
	maxBytes             int64
	file                 *os.File
	buf                  *bufio.Writer
	currentBytes         int64
	rotationCount        uint64
	lastRotationAt       time.Time
	lastArchivePath      string
	lastMaintenanceError string
}

func New(path string, maxMB int) (*Writer, error) {
	return NewWithOptions(Options{Path: path, MaxSizeMB: maxMB})
}

func NewWithOptions(opts Options) (*Writer, error) {
	if strings.TrimSpace(opts.Path) == "" {
		return nil, errors.New("file output path is required")
	}
	if opts.RetentionFiles < 0 || opts.RetentionDays < 0 {
		return nil, errors.New("file output retention values cannot be negative")
	}
	w := &Writer{opts: opts}
	if opts.MaxSizeMB > 0 {
		w.maxBytes = int64(opts.MaxSizeMB) * 1024 * 1024
	}
	if err := w.rotateClosedIfNeeded(); err != nil {
		return nil, err
	}
	if err := w.openAppend(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Writer) rotateClosedIfNeeded() error {
	if w.maxBytes <= 0 {
		return nil
	}
	info, err := os.Stat(w.opts.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Size() < w.maxBytes {
		return nil
	}
	archive, err := w.renameCurrent()
	if err != nil {
		return err
	}
	w.recordRotation(archive)
	w.runArchiveMaintenance(archive)
	return nil
}

func (w *Writer) openAppend() error {
	if err := os.MkdirAll(filepath.Dir(w.opts.Path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(w.opts.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	w.file = file
	w.buf = bufio.NewWriterSize(file, 256*1024)
	w.currentBytes = info.Size()
	return nil
}

func (w *Writer) WriteOne(value interface{}) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	record := append(encoded, '\n')

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil || w.buf == nil {
		return errors.New("file output writer is closed")
	}
	if w.maxBytes > 0 && w.currentBytes > 0 && w.currentBytes+int64(len(record)) > w.maxBytes {
		if err := w.rotateOpen(); err != nil {
			return err
		}
	}
	if _, err := w.buf.Write(record); err != nil {
		return err
	}
	w.currentBytes += int64(len(record))
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

func (w *Writer) rotateOpen() error {
	if err := w.buf.Flush(); err != nil {
		return fmt.Errorf("flush file output before rotation: %w", err)
	}
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("sync file output before rotation: %w", err)
	}
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("close file output before rotation: %w", err)
	}
	w.file = nil
	w.buf = nil

	archive, err := w.renameCurrent()
	if err != nil {
		_ = w.openAppend()
		return fmt.Errorf("rotate file output: %w", err)
	}
	if err := w.openAppend(); err != nil {
		return fmt.Errorf("reopen file output after rotating to %s: %w", archive, err)
	}
	w.recordRotation(archive)
	w.runArchiveMaintenance(archive)
	return nil
}

func (w *Writer) renameCurrent() (string, error) {
	if err := os.MkdirAll(filepath.Dir(w.opts.Path), 0o755); err != nil {
		return "", err
	}
	base := strings.TrimSuffix(w.opts.Path, filepath.Ext(w.opts.Path))
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	archive := fmt.Sprintf("%s_%s.log", base, stamp)
	for sequence := 1; ; sequence++ {
		if _, err := os.Stat(archive); errors.Is(err, os.ErrNotExist) {
			break
		} else if err != nil {
			return "", err
		}
		archive = fmt.Sprintf("%s_%s_%d.log", base, stamp, sequence)
	}
	if err := os.Rename(w.opts.Path, archive); err != nil {
		return "", err
	}
	return archive, nil
}

func (w *Writer) recordRotation(archive string) {
	w.rotationCount++
	w.lastRotationAt = time.Now().UTC()
	w.lastArchivePath = archive
	w.lastMaintenanceError = ""
}

func (w *Writer) runArchiveMaintenance(archive string) {
	if w.opts.CompressRotated {
		compressed, err := compressArchive(archive)
		if err != nil {
			w.lastMaintenanceError = err.Error()
		} else {
			archive = compressed
			w.lastArchivePath = compressed
		}
	}
	if err := w.pruneArchives(); err != nil {
		w.lastMaintenanceError = err.Error()
	}
}

func compressArchive(path string) (string, error) {
	source, err := os.Open(path)
	if err != nil {
		return "", err
	}

	target := path + ".gz"
	temp := target + ".tmp"
	output, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		_ = source.Close()
		return "", err
	}
	success := false
	defer func() {
		_ = source.Close()
		_ = output.Close()
		if !success {
			_ = os.Remove(temp)
		}
	}()

	compressor := gzip.NewWriter(output)
	if _, err := io.Copy(compressor, source); err != nil {
		_ = compressor.Close()
		return "", err
	}
	if err := source.Close(); err != nil {
		_ = compressor.Close()
		return "", err
	}
	if err := compressor.Close(); err != nil {
		return "", err
	}
	if err := output.Sync(); err != nil {
		return "", err
	}
	if err := output.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(temp, target); err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	success = true
	return target, nil
}

func (w *Writer) pruneArchives() error {
	if w.opts.RetentionFiles == 0 && w.opts.RetentionDays == 0 {
		return nil
	}
	base := strings.TrimSuffix(filepath.Base(w.opts.Path), filepath.Ext(w.opts.Path)) + "_"
	entries, err := os.ReadDir(filepath.Dir(w.opts.Path))
	if err != nil {
		return err
	}
	type archiveInfo struct {
		path    string
		modTime time.Time
	}
	archives := make([]archiveInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), base) ||
			(!strings.HasSuffix(entry.Name(), ".log") && !strings.HasSuffix(entry.Name(), ".log.gz")) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		archives = append(archives, archiveInfo{path: filepath.Join(filepath.Dir(w.opts.Path), entry.Name()), modTime: info.ModTime()})
	}
	sort.Slice(archives, func(i, j int) bool { return archives[i].modTime.After(archives[j].modTime) })

	var errs []error
	cutoff := time.Time{}
	if w.opts.RetentionDays > 0 {
		cutoff = time.Now().Add(-time.Duration(w.opts.RetentionDays) * 24 * time.Hour)
	}
	for index, archive := range archives {
		expiredByAge := !cutoff.IsZero() && archive.modTime.Before(cutoff)
		expiredByCount := w.opts.RetentionFiles > 0 && index >= w.opts.RetentionFiles
		if expiredByAge || expiredByCount {
			if err := os.Remove(archive.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (w *Writer) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buf == nil {
		return nil
	}
	return w.buf.Flush()
}

func (w *Writer) Status() Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	return Status{
		Path:                 w.opts.Path,
		CurrentBytes:         w.currentBytes,
		MaxBytes:             w.maxBytes,
		RetentionFiles:       w.opts.RetentionFiles,
		RetentionDays:        w.opts.RetentionDays,
		CompressRotated:      w.opts.CompressRotated,
		RotationCount:        w.rotationCount,
		LastRotationAt:       w.lastRotationAt,
		LastArchivePath:      w.lastArchivePath,
		LastMaintenanceError: w.lastMaintenanceError,
	}
}

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	var errs []error
	if w.buf != nil {
		errs = append(errs, w.buf.Flush())
	}
	if w.file != nil {
		errs = append(errs, w.file.Sync(), w.file.Close())
	}
	w.buf = nil
	w.file = nil
	return errors.Join(errs...)
}
