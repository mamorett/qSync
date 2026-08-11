package doctor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mamorett/qsync/internal/config"
	"github.com/mamorett/qsync/internal/lock"
)

// doctorMinFreeBytes is the free-space warning threshold (10 GiB).
const doctorMinFreeBytes = 10 * 1024 * 1024 * 1024

// Check is one diagnostic result.
type Check struct {
	Name    string `json:"check"`
	OK      bool   `json:"ok"`
	Warning bool   `json:"warning,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// Report is the full set of doctor checks.
type Report struct {
	Checks   []Check  `json:"checks"`
	Warnings []string `json:"warnings,omitempty"`
}

// HasHardFailure reports whether any non-warning check failed.
func (r *Report) HasHardFailure() bool {
	for _, c := range r.Checks {
		if !c.OK && !c.Warning {
			return true
		}
	}
	return false
}

// Run executes all doctor checks. cfgPath is the resolved config path; cfg may
// be nil if config loading failed (the first check reports that).
func Run(cfgPath string, cfg *config.Config, cfgErr error) *Report {
	r := &Report{}
	add := func(name string, ok, warn bool, detail string) {
		r.Checks = append(r.Checks, Check{Name: name, OK: ok, Warning: warn, Detail: detail})
		if warn && !ok {
			r.Warnings = append(r.Warnings, name+": "+detail)
		}
	}

	// 1. Config found and parses.
	if cfgErr != nil || cfg == nil {
		detail := "not found or invalid"
		if cfgErr != nil {
			detail = cfgErr.Error()
		}
		add("config", false, false, cfgPath+": "+detail)
		return r // nothing else is meaningful without config
	}
	add("config", true, false, cfgPath)

	defaultRsync := "rsync"
	if runtime.GOOS == "darwin" {
		if _, err := os.Stat("/opt/homebrew/bin/rsync"); err == nil {
			defaultRsync = "/opt/homebrew/bin/rsync"
		}
	}
	rsyncBin := binOrDefault(cfg.Transport.Rsync, defaultRsync)
	sshBin := binOrDefault(cfg.Transport.SSH, "ssh")

	// 2. rsync present + version.
	if path, err := exec.LookPath(rsyncBin); err != nil {
		add("rsync-binary", false, false, "not found: "+rsyncBin)
	} else {
		ver := firstLine(runCapture(path, "--version"))
		if runtime.GOOS == "darwin" && filepath.Clean(path) == "/usr/bin/rsync" {
			add("rsync-binary", true, true, ver+" (warning: macOS system rsync in use; brew rsync is recommended)")
		} else {
			add("rsync-binary", true, false, ver)
		}
	}

	// 3. ssh present.
	if _, err := exec.LookPath(sshBin); err != nil {
		add("ssh-binary", false, false, "not found: "+sshBin)
	} else {
		add("ssh-binary", true, false, sshBin)
	}

	// 4. SSH connectivity.
	connErr := runErr(sshBin, "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", cfg.Source.Host, "true")
	if connErr != nil {
		add("ssh-connectivity", false, false, connErr.Error())
	} else {
		add("ssh-connectivity", true, false, "connected to "+cfg.Source.Host)
	}

	// 5. Remote rsync present.
	if connErr == nil {
		if err := runErr(sshBin, cfg.Source.Host, "rsync", "--version"); err != nil {
			add("remote-rsync", false, false, "rsync not found on "+cfg.Source.Host)
		} else {
			add("remote-rsync", true, false, "present on "+cfg.Source.Host)
		}

		// 6. Remote source path readable.
		if err := runErr(sshBin, cfg.Source.Host, "test", "-r", cfg.Source.Path); err != nil {
			add("remote-source", false, false, cfg.Source.Path+" not readable on "+cfg.Source.Host)
		} else {
			add("remote-source", true, false, cfg.Source.Path)
		}

		// 11. Remote qsync binary.
		if err := runErr(sshBin, cfg.Source.Host, "command", "-v", "qsync"); err != nil {
			add("remote-qsync", false, true, "qsync not found on "+cfg.Source.Host+"; see README Remote Setup")
		} else {
			add("remote-qsync", true, false, "present on "+cfg.Source.Host)
		}
	}

	// 7. Local target exists or creatable (non-mutating).
	targetExists := false
	if fi, err := os.Stat(cfg.Target.Path); err == nil && fi.IsDir() {
		targetExists = true
		add("target-exists", true, false, cfg.Target.Path)
	} else {
		anc := nearestExistingAncestor(cfg.Target.Path)
		if anc != "" && isWritable(anc) {
			add("target-exists", true, true, "target missing but creatable under "+anc)
		} else {
			add("target-exists", false, false, "target missing and not creatable: "+cfg.Target.Path)
		}
	}

	// 8. Target writable (only if it exists).
	if targetExists {
		if err := probeWritable(cfg.Target.Path); err != nil {
			add("target-writable", false, false, err.Error())
		} else {
			add("target-writable", true, false, "writable")
		}

		// 9. Free disk space.
		free, err := freeBytes(cfg.Target.Path)
		if err != nil {
			add("free-space", true, true, "could not determine: "+err.Error())
		} else if free < doctorMinFreeBytes {
			add("free-space", false, true, fmt.Sprintf("low free space: %d bytes", free))
		} else {
			add("free-space", true, false, fmt.Sprintf("%d bytes free", free))
		}

		// 10. Lock status.
		held, info, err := lock.IsLocked(cfg.Target.Path)
		switch {
		case err != nil:
			add("lock-status", true, true, "could not check: "+err.Error())
		case held:
			add("lock-status", false, false, fmt.Sprintf("lock held by pid %d (%s)", info.PID, info.Operation))
		default:
			add("lock-status", true, false, "not locked")
		}
	} else {
		add("target-writable", true, true, "skipped: target does not exist")
	}

	return r
}

func binOrDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

func runCapture(name string, args ...string) string {
	var buf bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	_ = cmd.Run()
	return buf.String()
}

func runErr(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg != "" {
			return fmt.Errorf("%s", firstLine(msg))
		}
		return err
	}
	return nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func nearestExistingAncestor(p string) string {
	cur := filepath.Clean(p)
	for {
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		if fi, err := os.Stat(parent); err == nil && fi.IsDir() {
			return parent
		}
		cur = parent
	}
}

func isWritable(dir string) bool {
	return probeWritable(dir) == nil
}

func probeWritable(dir string) error {
	tmpDir := filepath.Join(dir, ".qsync", "tmp")
	// Only create tmp under an existing target's .qsync; use dir itself if
	// .qsync doesn't exist yet.
	base := dir
	if fi, err := os.Stat(filepath.Join(dir, ".qsync")); err == nil && fi.IsDir() {
		if err := os.MkdirAll(tmpDir, 0755); err == nil {
			base = tmpDir
		}
	}
	f, err := os.CreateTemp(base, ".qsync-doctor-*")
	if err != nil {
		return fmt.Errorf("not writable: %s", base)
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return nil
}
