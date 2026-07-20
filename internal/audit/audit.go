package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/yourorg/qsync/internal/planner"
)

// Record is one audit entry. A run writes one summary record followed by
// per-file records.
type Record struct {
	Timestamp        time.Time     `json:"timestamp"`
	Hostname         string        `json:"hostname"`
	User             string        `json:"user"`
	Operation        string        `json:"operation"`
	DryRun           bool          `json:"dry_run"`
	RsyncArgv        []string      `json:"rsync_argv,omitempty"`
	RsyncExit        int           `json:"rsync_exit"`
	FilesChanged     int           `json:"files_changed"`
	BytesTransferred int64         `json:"bytes_transferred"`
	Plan             *planner.Plan `json:"plan,omitempty"`
	Result           string        `json:"result"`
	DurationMs       int64         `json:"duration_ms"`
	// Path is set on per-file detail records.
	Path string `json:"path,omitempty"`
}

// Writer appends JSONL records to a per-run audit file.
type Writer struct {
	f  *os.File
	bw *bufio.Writer
}

// HistoryDir returns the history directory for a target path.
func HistoryDir(targetPath string) string {
	return filepath.Join(targetPath, ".qsync", "history")
}

// NewWriter creates a new audit file <target>/.qsync/history/YYYYMMDD-HHMMSS-<op>.jsonl.
// Filenames use UTC.
func NewWriter(targetPath, operation string, when time.Time) (*Writer, error) {
	dir := HistoryDir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%s-%s.jsonl", when.UTC().Format("20060102-150405"), operation)
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &Writer{f: f, bw: bufio.NewWriter(f)}, nil
}

// Append writes one record as a JSON line.
func (w *Writer) Append(r Record) error {
	if r.Timestamp.IsZero() {
		r.Timestamp = time.Now().UTC()
	}
	if r.Hostname == "" {
		r.Hostname, _ = os.Hostname()
	}
	if r.User == "" {
		if u, err := user.Current(); err == nil {
			r.User = u.Username
		}
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if _, err := w.bw.Write(b); err != nil {
		return err
	}
	return w.bw.WriteByte('\n')
}

// Close flushes and closes the audit file.
func (w *Writer) Close() error {
	if w == nil || w.f == nil {
		return nil
	}
	if err := w.bw.Flush(); err != nil {
		w.f.Close()
		return err
	}
	err := w.f.Close()
	w.f = nil
	return err
}

// ReadFile reads all records from an audit file (for testing/inspection).
func ReadFile(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var recs []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var r Record
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, err
		}
		recs = append(recs, r)
	}
	return recs, sc.Err()
}
