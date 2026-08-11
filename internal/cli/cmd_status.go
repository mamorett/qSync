package cli

import (
	"fmt"
	"time"

	"github.com/mamorett/qsync/internal/exitcode"
	"github.com/mamorett/qsync/internal/lock"
	"github.com/mamorett/qsync/internal/snapshot"
)

type statusData struct {
	LastSync     string `json:"last_sync"`
	LocalAdds    int    `json:"local_adds"`
	LocalUpdates int    `json:"local_updates"`
	LocalDeletes int    `json:"local_deletes"`
	Locked       bool   `json:"locked"`
	Config       string `json:"config"`
}

func cmdStatus(e *env) (exitcode.ExitCode, error) {
	if wantsHelp(e.args) {
		fmt.Fprint(e.stdout, helpText(commands["status"]))
		return exitcode.Success, nil
	}
	fs, g := newFlagSet("status", e)
	if err := fs.Parse(e.args); err != nil {
		return exitcode.GenericError, e.parseErr(commands["status"], err)
	}

	cfg, cfgPath, err := e.loadConfig()
	if err != nil {
		return exitcode.GenericError, err
	}

	sp := stateFor(cfg.Target.Path)
	synced, haveSynced, err := loadSynced(sp)
	if err != nil {
		return exitcode.GenericError, err
	}

	// Fresh local scan (cheap, read-only).
	res, err := snapshot.Scan(cfg.Target.Path, cfg.Ignore)
	if err != nil {
		return exitcode.GenericError, err
	}
	local := res.Manifest

	diff := synced.Diff(local)

	lastSync := "never"
	if haveSynced && !synced.Generated.IsZero() {
		lastSync = synced.Generated.UTC().Format(time.RFC3339)
	}

	locked, _, _ := lock.IsLocked(cfg.Target.Path)

	data := statusData{
		LastSync:     lastSync,
		LocalAdds:    len(diff.Added),
		LocalUpdates: len(diff.Updated),
		LocalDeletes: len(diff.Deleted),
		Locked:       locked,
		Config:       cfgPath,
	}

	pending := data.LocalAdds+data.LocalUpdates+data.LocalDeletes > 0
	code := exitcode.Success
	if pending {
		code = exitcode.PendingChanges
	}

	if g.json {
		if err := emitJSON(e.stdout, "status", code, nil, data, "", nil); err != nil {
			return exitcode.GenericError, err
		}
	} else if !g.quiet {
		fmt.Fprintf(e.stdout, "Last sync: %s\n", lastSync)
		fmt.Fprintf(e.stdout, "Local changes since sync: %d added, %d updated, %d removed\n",
			data.LocalAdds, data.LocalUpdates, data.LocalDeletes)
		fmt.Fprintf(e.stdout, "Lock: %s\n", lockedStr(locked))
		fmt.Fprintf(e.stdout, "Config: %s\n", cfgPath)
	}

	if pending {
		return code, ErrPending
	}
	return code, nil
}

func lockedStr(b bool) string {
	if b {
		return "held"
	}
	return "free"
}
