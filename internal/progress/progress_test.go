package progress

import (
	"testing"
	"time"
)

func TestProgressBar_TerminalAndNonTerminal(t *testing.T) {
	// Non-terminal test
	pb := &ProgressBar{
		total:            5,
		description:      "Testing",
		startTime:        time.Now(),
		lastRedraw:       time.Unix(0, 0),
		throttleDuration: 0,
		isTerminal:       false,
	}

	pb.UpdateCopy("photo1.jpg", "/local/photo1.jpg")
	if pb.copied != 1 || pb.current != 1 || pb.errors != 0 {
		t.Errorf("unexpected counters after UpdateCopy: copied=%d current=%d errors=%d", pb.copied, pb.current, pb.errors)
	}

	pb.UpdateError("photo2.jpg", "/local/photo2.jpg", "permission denied")
	if pb.copied != 1 || pb.current != 2 || pb.errors != 1 {
		t.Errorf("unexpected counters after UpdateError: copied=%d current=%d errors=%d", pb.copied, pb.current, pb.errors)
	}

	pb.Finish()
}

func TestProgressBar_CountersAndMethods(t *testing.T) {
	pb := NewProgressBar(10, "Test")
	pb.throttleDuration = 0

	pb.UpdateCopy("a.txt", "/src/a.txt")
	if pb.copied != 1 || pb.current != 1 || pb.currentSource != "/src/a.txt" {
		t.Errorf("UpdateCopy mismatch: copied=%d, src=%s", pb.copied, pb.currentSource)
	}

	pb.SetCurrentFile("b.txt", "/src/b.txt")
	if pb.currentSource != "/src/b.txt" {
		t.Errorf("SetCurrentFile mismatch: src=%s", pb.currentSource)
	}

	pb.Increment()
	if pb.copied != 2 || pb.current != 2 {
		t.Errorf("Increment mismatch: copied=%d, current=%d", pb.copied, pb.current)
	}

	pb.Finish()
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "h...d"},
		{"abcdefghij", 7, "ab...ij"},
	}

	for _, tt := range tests {
		got := truncateString(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestFormatTime(t *testing.T) {
	if got := formatTime(65 * time.Second); got != "01:05" {
		t.Errorf("formatTime(65s) = %q, want 01:05", got)
	}
	if got := formatTime(3665 * time.Second); got != "01:01:05" {
		t.Errorf("formatTime(3665s) = %q, want 01:01:05", got)
	}
}
