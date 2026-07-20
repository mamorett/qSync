package cli

import (
	"io"
	"sync"
)

// syncWriter serializes concurrent writes to an underlying writer. rsync
// streaming writes to stderr from two goroutines (the child's stderr copy and
// our stdout line parser), so both must share one lock.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func newSyncWriter(w io.Writer) *syncWriter { return &syncWriter{w: w} }

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
