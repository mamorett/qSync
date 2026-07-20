package lock

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"time"
)

// LockFileName is the lock file name inside .qsync.
const LockFileName = "sync.lock"

// Info is the JSON payload written into the lock file.
type Info struct {
	PID       int    `json:"pid"`
	Host      string `json:"host"`
	User      string `json:"user"`
	Started   string `json:"started"`
	Operation string `json:"operation"`
}

// ErrLocked indicates the lock is held by another process.
type ErrLocked struct {
	Holder Info
	Raw    string
}

func (e *ErrLocked) Error() string {
	if e.Holder.PID != 0 {
		return fmt.Sprintf("lock held by pid %d on %s (operation %q, started %s)",
			e.Holder.PID, e.Holder.Host, e.Holder.Operation, e.Holder.Started)
	}
	if e.Raw != "" {
		return "lock held: " + e.Raw
	}
	return "lock held by another process"
}

// IsErrLocked reports whether err is an ErrLocked.
func IsErrLocked(err error) bool {
	var el *ErrLocked
	return errors.As(err, &el)
}

// Lock is an acquired advisory lock.
type Lock struct {
	f    *os.File
	path string
}

// LockPath returns the lock file path for a target library root.
func LockPath(targetPath string) string {
	return filepath.Join(targetPath, ".qsync", LockFileName)
}

// Acquire opens and non-blocking-locks the lock file for the target path,
// writing operation metadata into it. On contention it returns *ErrLocked.
func Acquire(targetPath, operation string) (*Lock, error) {
	path := LockPath(targetPath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}

	if err := flockNB(f); err != nil {
		// Could not lock. Inspect current holder.
		data, _ := os.ReadFile(path)
		f.Close()
		info := parseInfo(data)
		if stale(info) {
			// Take over: reopen and retry once (flock still serializes takeover).
			return acquireTakeover(path, operation)
		}
		return nil, &ErrLocked{Holder: info, Raw: string(data)}
	}

	if err := writeInfo(f, operation); err != nil {
		unflock(f)
		f.Close()
		return nil, err
	}
	return &Lock{f: f, path: path}, nil
}

func acquireTakeover(path, operation string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := flockNB(f); err != nil {
		data, _ := os.ReadFile(path)
		f.Close()
		return nil, &ErrLocked{Holder: parseInfo(data), Raw: string(data)}
	}
	if err := writeInfo(f, operation); err != nil {
		unflock(f)
		f.Close()
		return nil, err
	}
	return &Lock{f: f, path: path}, nil
}

// Release clears the lock file contents (without unlinking, to avoid
// unlink/recreate races), unlocks, and closes.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	// Clear contents.
	_ = l.f.Truncate(0)
	_, _ = l.f.Seek(0, 0)
	unflock(l.f)
	err := l.f.Close()
	l.f = nil
	return err
}

// IsLocked reports whether the lock for the target is currently held by another
// live process. It attempts a non-blocking lock and immediately releases it.
func IsLocked(targetPath string) (bool, Info, error) {
	path := LockPath(targetPath)
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		if os.IsNotExist(err) {
			return false, Info{}, nil
		}
		return false, Info{}, err
	}
	defer f.Close()

	if err := flockNB(f); err != nil {
		data, _ := os.ReadFile(path)
		info := parseInfo(data)
		if stale(info) {
			return false, info, nil
		}
		return true, info, nil
	}
	// We got the lock, so nobody holds it. Release immediately.
	unflock(f)
	return false, Info{}, nil
}

func writeInfo(f *os.File, operation string) error {
	host, _ := os.Hostname()
	uname := ""
	if u, err := user.Current(); err == nil {
		uname = u.Username
	}
	info := Info{
		PID:       os.Getpid(),
		Host:      host,
		User:      uname,
		Started:   time.Now().UTC().Format(time.RFC3339),
		Operation: operation,
	}
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}

func parseInfo(data []byte) Info {
	var info Info
	_ = json.Unmarshal(data, &info)
	return info
}

// stale reports whether the lock's recorded PID is no longer alive. Best-effort:
// PID checks are unreliable across reboots but flock is the real mutex.
func stale(info Info) bool {
	if info.PID == 0 {
		return false
	}
	return !processAlive(info.PID)
}
