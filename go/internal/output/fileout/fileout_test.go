package fileout

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type testRecord struct {
	Sequence int    `json:"sequence"`
	Payload  string `json:"payload"`
}

func TestWriterRotatesWhileProcessRemainsOpenWithoutLosingRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ping_results.log")
	writer, err := NewWithOptions(Options{Path: path, MaxSizeMB: 1})
	if err != nil {
		t.Fatal(err)
	}
	payload := strings.Repeat("x", 600*1024)
	for sequence := 1; sequence <= 4; sequence++ {
		if err := writer.WriteOne(testRecord{Sequence: sequence, Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	status := writer.Status()
	if status.RotationCount < 2 {
		t.Fatalf("rotation count = %d, want at least 2", status.RotationCount)
	}
	if status.CurrentBytes > status.MaxBytes {
		t.Fatalf("current bytes = %d, max = %d", status.CurrentBytes, status.MaxBytes)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	sequences := readSequences(t, dir)
	if got := strings.Trim(strings.Join(sequences, ","), ","); got != "1,2,3,4" {
		t.Fatalf("record sequence = %s, want 1,2,3,4", got)
	}
}

func TestWriterCompressesAndPrunesArchives(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ping_results.log")
	writer, err := NewWithOptions(Options{
		Path: path, MaxSizeMB: 1, RetentionFiles: 2, CompressRotated: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	writer.mu.Lock()
	writer.maxBytes = 200
	writer.mu.Unlock()
	for sequence := 1; sequence <= 8; sequence++ {
		if err := writer.WriteOne(testRecord{Sequence: sequence, Payload: strings.Repeat("x", 150)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	archives, err := filepath.Glob(filepath.Join(dir, "ping_results_*.log.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if len(archives) != 2 {
		t.Fatalf("compressed archive count = %d, want 2", len(archives))
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, "ping_results_*.log")); len(leftovers) != 0 {
		t.Fatalf("uncompressed archives remain: %v", leftovers)
	}
}

func TestWriterRotatesOversizedFileAtStartup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ping_results.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 1024*1024+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	writer, err := NewWithOptions(Options{Path: path, MaxSizeMB: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteOne(testRecord{Sequence: 1}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	archives, err := filepath.Glob(filepath.Join(dir, "ping_results_*.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(archives) != 1 {
		t.Fatalf("archive count = %d, want 1", len(archives))
	}
}

func TestWriterRotatesAtConfiguredFiftyMegabytesUnderDiscoveryVolume(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ping_results.log")
	writer, err := NewWithOptions(Options{Path: path, MaxSizeMB: 50, RetentionFiles: 4})
	if err != nil {
		t.Fatal(err)
	}
	payload := strings.Repeat("d", 1024*1024-256)
	const records = 55
	for sequence := 1; sequence <= records; sequence++ {
		if err := writer.WriteOne(testRecord{Sequence: sequence, Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	status := writer.Status()
	if status.RotationCount != 1 {
		t.Fatalf("rotation count = %d, want 1 at the configured 50 MB boundary", status.RotationCount)
	}
	if status.CurrentBytes > status.MaxBytes {
		t.Fatalf("active log bytes = %d, configured max = %d", status.CurrentBytes, status.MaxBytes)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(dir, "ping_results*.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("log files = %d, want active + one rotated archive", len(paths))
	}
	lineCount := 0
	for _, currentPath := range paths {
		file, openErr := os.Open(currentPath)
		if openErr != nil {
			t.Fatal(openErr)
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
		for scanner.Scan() {
			lineCount++
		}
		if scanErr := scanner.Err(); scanErr != nil {
			_ = file.Close()
			t.Fatal(scanErr)
		}
		_ = file.Close()
	}
	if lineCount != records {
		t.Fatalf("records across active and rotated logs = %d, want %d", lineCount, records)
	}
}

func readSequences(t *testing.T, dir string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "ping_results*"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	sequences := make([]string, 0)
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		var reader io.Reader = file
		var zipped *gzip.Reader
		if strings.HasSuffix(path, ".gz") {
			zipped, err = gzip.NewReader(file)
			if err != nil {
				_ = file.Close()
				t.Fatal(err)
			}
			reader = zipped
		}
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
		for scanner.Scan() {
			var record testRecord
			if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
				continue // startup oversized fixture is not JSON
			}
			sequences = append(sequences, string(rune('0'+record.Sequence)))
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		if zipped != nil {
			_ = zipped.Close()
		}
		_ = file.Close()
	}
	sort.Strings(sequences)
	return sequences
}
