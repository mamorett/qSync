package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Symbols for human output
type Symbol string

const (
	SymbolSuccess   Symbol = "✓"
	SymbolAdd       Symbol = "+"
	SymbolUpdate    Symbol = "~"
	SymbolDelete    Symbol = "-"
	SymbolConflict  Symbol = "!"
	SymbolDirection Symbol = "→"
)

// HumanizeIEC converts bytes to human-readable format with IEC units
type HumanizeIEC int64

const (
	B  = 1
	Ki = 1024
	Mi = 1024 * Ki
	Gi = 1024 * Mi
)

func (v HumanizeIEC) String() string {
	n := int64(v)
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < Mi {
		return fmt.Sprintf("%.1f KiB", float64(n)/float64(Ki))
	}
	if n < Gi {
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(Mi))
	}
	return fmt.Sprintf("%.1f GiB", float64(n)/float64(Gi))
}

// WriteHeader writes a bold-free section header
func WriteHeader(w io.Writer, title string) {
	fmt.Fprintf(w, "%s\n", title)
}

// WriteLine writes a line with symbol prefix
func WriteLine(w io.Writer, symbol Symbol, msg string) {
	fmt.Fprintf(w, "  %s %s\n", symbol, msg)
}

// Error writes error to output
func Error(w io.Writer, format string, a ...any) {
	fmt.Fprintf(w, "error: %s\n", fmt.Sprintf(format, a...))
}

// Warn writes warning to output
func Warn(w io.Writer, format string, a ...any) {
	fmt.Fprintf(w, "warning: %s\n", fmt.Sprintf(format, a...))
}

// Info writes information to output
func Info(w io.Writer, format string, a ...any) {
	fmt.Fprintf(w, "%s\n", fmt.Sprintf(format, a...))
}

// JSONEmitter handles JSON document output
type JSONEmitter struct {
	encoder *json.Encoder
	dest    io.Writer
}

func NewJSONEmitter(w io.Writer) *JSONEmitter {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return &JSONEmitter{
		encoder: enc,
		dest:    w,
	}
}

func (je *JSONEmitter) EmitEnvelope(cmd string, ok bool, exitCode int, warnings []string, data any) error {
	result := map[string]any{
		"qsync":        1,
		"command":      cmd,
		"ok":           ok,
		"exit_code":    exitCode,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	}

	if len(warnings) > 0 {
		result["warnings"] = warnings
	}

	if data != nil {
		result["data"] = data
	}

	return je.encoder.Encode(result)
}

func (JSONEmitter) IsJSONOutputEnabled() bool {
	// Detect via CLI parsing - not implemented yet, always returns true for testing
	return true
}

// WriteErrorJSON formats an error response
func (je *JSONEmitter) WriteError(cmd string, exitCode int, msg string, details any) error {
	result := map[string]any{
		"qsync":     1,
		"command":   cmd,
		"ok":        false,
		"exit_code": exitCode,
		"error": map[string]any{
			"message": msg,
		},
	}

	if details != nil {
		result["error"].(map[string]any)["details"] = details
	}

	return je.encoder.Encode(result)
}

// TTY detection
type NoTTY struct{}

func (n NoTTY) IsTerminal() bool {
	// Check if stdout is a terminal
	stat, _ := os.Stdout.Stat()
	return (stat.Mode() & os.ModeCharDevice) != 0
}

var IsTTY = NoTTY{}

// ShellQuote safely quotes a path for shell usage
func ShellQuote(path string) string {
	// Simple implementation - improve if special chars are found
	if strings.ContainsAny(path, " \t\n'\"") {
		return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
	}
	return path
}
