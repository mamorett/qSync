package progress

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// ProgressBar wrapper for terminal progress with 3-line display:
// Line 1: Progress bar with percentage, file count, speed, ETA
// Line 2: Current file being copied with its source path
// Line 3: Status with copied files, remaining files, and error count
type ProgressBar struct {
	total            int
	current          int
	copied           int
	deleted          int
	errors           int
	description      string
	currentFile      string
	currentSource    string
	startTime        time.Time
	lastRedraw       time.Time
	throttleDuration time.Duration
	isTerminal       bool
	mu               sync.Mutex
}

// NewProgressBar creates a new progress bar
func NewProgressBar(total int, description string) *ProgressBar {
	isTerm := term.IsTerminal(int(os.Stdout.Fd()))
	pb := &ProgressBar{
		total:            total,
		description:      description,
		startTime:        time.Now(),
		lastRedraw:       time.Unix(0, 0),
		throttleDuration: 65 * time.Millisecond,
		isTerminal:       isTerm,
	}
	pb.drawNoLock()
	return pb
}

func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return 80 // fallback
	}
	return width
}

func formatTime(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	half := (maxLen - 3) / 2
	return string(runes[:half]) + "..." + string(runes[len(runes)-half:])
}

func (pb *ProgressBar) drawNoLock() {
	remaining := pb.total - pb.current
	if remaining < 0 {
		remaining = 0
	}

	if !pb.isTerminal {
		// For non-terminal, log at most once every 5 seconds or on completion
		if pb.current == pb.total || time.Since(pb.lastRedraw) >= 5*time.Second {
			pct := 0
			if pb.total > 0 {
				pct = int(float64(pb.current) * 100 / float64(pb.total))
			}
			srcDisplay := pb.currentSource
			if srcDisplay == "" {
				srcDisplay = pb.currentFile
			}
			if srcDisplay != "" {
				fmt.Printf("⚙ %s... %d%% (%d/%d) - Active: %s - Status: %d copied, %d deleted, %d remaining, %d errors\n",
					pb.description, pct, pb.current, pb.total, srcDisplay, pb.copied, pb.deleted, remaining, pb.errors)
			} else {
				fmt.Printf("⚙ %s... %d%% (%d/%d) - Status: %d copied, %d deleted, %d remaining, %d errors\n",
					pb.description, pct, pb.current, pb.total, pb.copied, pb.deleted, remaining, pb.errors)
			}
			pb.lastRedraw = time.Now()
		}
		return
	}

	termWidth := getTerminalWidth()

	// --- Line 1: Progress Bar ---
	pct := 0
	if pb.total > 0 {
		pct = int(float64(pb.current) * 100 / float64(pb.total))
	}
	pctStr := fmt.Sprintf("%3d%% ", pct)
	pctLen := len(pctStr)

	elapsed := time.Since(pb.startTime)
	speed := 0
	if elapsed.Seconds() > 0.1 {
		speed = int(float64(pb.current) / elapsed.Seconds())
	}

	totalDigits := len(fmt.Sprintf("%d", pb.total))
	if totalDigits < 1 {
		totalDigits = 1
	}

	countersFormat := fmt.Sprintf(" (%%%dd/%%d, %%d img/s) ", totalDigits)
	countersStr := fmt.Sprintf(countersFormat, pb.current, pb.total, speed)
	countersLen := len(countersStr)

	var estRemaining time.Duration
	if pb.current > 0 && pb.total > pb.current {
		estRemaining = time.Duration(float64(elapsed) / float64(pb.current) * float64(pb.total-pb.current))
	}
	timeStr := fmt.Sprintf("[%s<%s]", formatTime(elapsed), formatTime(estRemaining))
	timeLen := len(timeStr)

	prefixText := "⚙ " + pb.description + "... "
	prefixLen := 2 + len(pb.description) + 4

	barWidth := 30
	showBar := true

	needed := (prefixLen + pctLen + (barWidth + 2) + countersLen + timeLen) - (termWidth - 2)
	if needed > 0 {
		shrinkage := needed
		if barWidth-shrinkage >= 10 {
			barWidth -= shrinkage
			needed = 0
		} else {
			needed -= (barWidth - 10)
			barWidth = 10
		}
	}

	if needed > 0 {
		countersFormatShort := fmt.Sprintf(" (%%%dd/%%d) ", totalDigits)
		countersStr = fmt.Sprintf(countersFormatShort, pb.current, pb.total)
		oldLen := countersLen
		countersLen = len(countersStr)
		needed -= (oldLen - countersLen)
	}

	if needed > 0 {
		needed -= timeLen
		timeStr = ""
		timeLen = 0
	}

	if needed > 0 {
		desc := pb.description
		descLimit := len(desc) - needed
		if descLimit < 4 {
			descLimit = 4
		}
		if descLimit < len(desc) {
			desc = desc[:descLimit]
			prefixText = "⚙ " + desc + "... "
			oldLen := prefixLen
			prefixLen = 2 + len(desc) + 4
			needed -= (oldLen - prefixLen)
		}
	}

	if needed > 0 {
		showBar = false
		needed -= (barWidth + 2)
	}

	var line1 string
	line1 += "\033[36m" + prefixText + "\033[0m"
	line1 += pctStr

	if showBar {
		filled := 0
		if pb.total > 0 {
			filled = int(float64(pb.current) / float64(pb.total) * float64(barWidth))
		}
		if filled < 0 {
			filled = 0
		}
		if filled > barWidth {
			filled = barWidth
		}

		line1 += "\033[36m▕\033[0m"
		line1 += "\033[36m" + strings.Repeat("█", filled) + "\033[0m"
		line1 += strings.Repeat("░", barWidth-filled)
		line1 += "\033[36m▏\033[0m"
	}

	line1 += countersStr
	if timeStr != "" {
		line1 += timeStr
	}

	// --- Line 2: Current File being copied/deleted with its source path ---
	maxLineLen := termWidth - 4
	if maxLineLen < 20 {
		maxLineLen = 20
	}

	fileDisplay := pb.currentSource
	if fileDisplay == "" {
		fileDisplay = pb.currentFile
	}
	if fileDisplay == "" {
		fileDisplay = "..."
	}

	line2Prefix := "Processing: "
	avail := maxLineLen - len([]rune(line2Prefix))
	if avail < 10 {
		avail = 10
	}

	truncatedSrc := fileDisplay
	if len([]rune(fileDisplay)) > avail {
		truncatedSrc = truncateString(fileDisplay, avail)
	}
	line2 := "\033[36m" + line2Prefix + "\033[0m\033[37m" + truncatedSrc + "\033[0m"

	// --- Line 3: Status (copied, deleted, remaining, errors) ---
	errColor := "\033[32m"
	if pb.errors > 0 {
		errColor = "\033[31m"
	}
	line3 := fmt.Sprintf("\033[1mStatus:\033[0m \033[32m%d copied\033[0m | \033[35m%d deleted\033[0m | \033[33m%d remaining\033[0m | %s%d errors\033[0m",
		pb.copied, pb.deleted, remaining, errColor, pb.errors)

	// Redraw 3 lines: clear line 1, print line 1 \n, clear line 2, print line 2 \n, clear line 3, print line 3, move cursor UP 2 lines
	fmt.Printf("\r\033[2K%s\n\033[2K%s\n\033[2K%s\033[2A\r", line1, line2, line3)
	pb.lastRedraw = time.Now()
}

// UpdateCopy records a successfully copied file with its source path and increments progress
func (pb *ProgressBar) UpdateCopy(relPath, srcPath string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.currentFile = relPath
	pb.currentSource = srcPath
	pb.copied++
	pb.current++
	if time.Since(pb.lastRedraw) >= pb.throttleDuration || pb.current == pb.total {
		pb.drawNoLock()
	}
}

// UpdateDelete records a successfully deleted file and increments progress
func (pb *ProgressBar) UpdateDelete(relPath, srcPath string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.currentFile = relPath
	pb.currentSource = srcPath
	pb.deleted++
	pb.current++
	if time.Since(pb.lastRedraw) >= pb.throttleDuration || pb.current == pb.total {
		pb.drawNoLock()
	}
}

// UpdateError records an error during copy/delete
func (pb *ProgressBar) UpdateError(relPath, srcPath string, errStr string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	if relPath != "" {
		pb.currentFile = relPath
	}
	if srcPath != "" {
		pb.currentSource = srcPath
	}
	pb.errors++
	pb.current++
	if time.Since(pb.lastRedraw) >= pb.throttleDuration || pb.current == pb.total {
		pb.drawNoLock()
	}
}

// SetCurrentFile sets current file and source path without incrementing counters
func (pb *ProgressBar) SetCurrentFile(relPath, srcPath string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.currentFile = relPath
	pb.currentSource = srcPath
	if time.Since(pb.lastRedraw) >= pb.throttleDuration {
		pb.drawNoLock()
	}
}

// LogAbove prints a message above the active progress display without corrupting lines
func (pb *ProgressBar) LogAbove(msg string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	if !pb.isTerminal {
		fmt.Fprintln(os.Stderr, msg)
		return
	}

	// Clear the 3 progress lines, print msg above, and redraw
	fmt.Print("\r\033[2K\n\033[2K\n\033[2K\033[2A\r")
	fmt.Fprintln(os.Stderr, msg)
	pb.drawNoLock()
}

// UpdateWithStatus updates description and increments progress
func (pb *ProgressBar) UpdateWithStatus(status string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	pb.currentFile = status
	pb.currentSource = status
	pb.copied++
	pb.current++
	pb.drawNoLock()
}

// Increment just increments progress
func (pb *ProgressBar) Increment() {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.copied++
	pb.current++
	if time.Since(pb.lastRedraw) >= pb.throttleDuration || pb.current == pb.total {
		pb.drawNoLock()
	}
}

// IncrementWithStatus updates description and increments progress
func (pb *ProgressBar) IncrementWithStatus(status string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.currentFile = status
	pb.copied++
	pb.current++
	if time.Since(pb.lastRedraw) >= pb.throttleDuration || pb.current == pb.total {
		pb.drawNoLock()
	}
}

// Describe sets description without incrementing
func (pb *ProgressBar) Describe(desc string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.currentFile = desc
	if time.Since(pb.lastRedraw) >= pb.throttleDuration {
		pb.drawNoLock()
	}
}

// Finish finishes the progress bar display
func (pb *ProgressBar) Finish() {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	remaining := pb.total - pb.current
	if remaining < 0 {
		remaining = 0
	}

	if !pb.isTerminal {
		pct := 0
		if pb.total > 0 {
			pct = int(float64(pb.current) * 100 / float64(pb.total))
		}
		fmt.Printf("⚙ %s... %d%% (%d/%d) - Complete (Copied: %d, Deleted: %d, Remaining: %d, Errors: %d)\n",
			pb.description, pct, pb.current, pb.total, pb.copied, pb.deleted, remaining, pb.errors)
		return
	}

	pb.drawNoLock()
	fmt.Printf("\n\n\n")
}
