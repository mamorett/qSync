package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yourorg/photolib/internal/audit"
	"github.com/yourorg/photolib/internal/config"
	"github.com/yourorg/photolib/internal/exitcode"
	"github.com/yourorg/photolib/internal/lock"
	"github.com/yourorg/photolib/internal/planner"
	"github.com/yourorg/photolib/internal/purge"
	"github.com/yourorg/photolib/internal/rsyncx"
	"github.com/yourorg/photolib/internal/snapshot"
)

// syncOptions configure a pull/push run.
type syncOptions struct {
	direction planner.Direction
	apply     bool
	all       bool
	waitLock  time.Duration
}

// runSync executes the shared pull/push flow.
func runSync(e *env, opname string, opts syncOptions) (exitcode.ExitCode, error) {
	g := &e.globals
	cfg, _, err := e.loadConfig()
	if err != nil {
		return exitcode.GenericError, err
	}

	// Acquire lock (held for the entire operation incl. planning).
	lk, err := acquireWithWait(cfg.Target.Path, opname, opts.waitLock)
	if err != nil {
		if lock.IsErrLocked(err) {
			if g.json {
				return exitcode.LockActive, emitJSON(e.stdout, opname, exitcode.LockActive, nil, nil, err.Error(), nil)
			}
			fmt.Fprintf(e.stderr, "error: %v\n", err)
			return exitcode.LockActive, ErrLockActive
		}
		return exitcode.GenericError, err
	}
	defer lk.Release()

	// Signal handler: release lock and exit 130 on interrupt.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	start := time.Now()
	aw, _ := audit.NewWriter(cfg.Target.Path, opname, start)
	if aw != nil {
		defer aw.Close()
	}

	bc, scanErrs, err := gatherAndPlan(cfg, opts.direction)
	if err != nil {
		writeAuditSummary(aw, opname, opts.apply, nil, 1, "error: "+err.Error(), start)
		return exitcode.GenericError, err
	}
	plan := bc.plan

	// Scan errors abort apply.
	if len(scanErrs) > 0 && opts.apply {
		for _, se := range scanErrs {
			fmt.Fprintf(e.stderr, "scan error: %s\n", se.Error())
		}
		writeAuditSummary(aw, opname, opts.apply, plan, 1, "error: scan errors", start)
		return exitcode.GenericError, fmt.Errorf("scan errors present; aborting")
	}

	// Conflicts abort.
	if plan.Stats.Conflicts > 0 {
		writeAuditSummary(aw, opname, opts.apply, plan, 0, "conflicts", start)
		if g.json {
			return exitcode.Conflicts, emitJSON(e.stdout, opname, exitcode.Conflicts, nil, plan, "conflicts detected", plan.Conflicts)
		}
		renderPlanHuman(e.stdout, plan, opts.all)
		return exitcode.Conflicts, ErrConflicts
	}

	// Push Safety Rule 4: refuse if the DGX has newer changes the local lacks.
	if opts.direction == planner.DirectionPush {
		if violated := remoteHasNewerChanges(bc); violated {
			msg := "DGX has newer changes; run photolib pull first"
			writeAuditSummary(aw, opname, opts.apply, plan, 0, "conflicts", start)
			if g.json {
				return exitcode.Conflicts, emitJSON(e.stdout, opname, exitcode.Conflicts, nil, plan, msg, nil)
			}
			fmt.Fprintf(e.stderr, "error: %s\n", msg)
			return exitcode.Conflicts, ErrConflicts
		}
	}

	argv := rsyncx.BuildArgs(opts.direction, cfg, true)
	binary := rsyncx.Binary(cfg)

	// Dry-run path.
	if !opts.apply {
		if g.json {
			data := map[string]interface{}{"plan": plan, "rsync_argv": append([]string{binary}, argv...), "dry_run": true}
			code, sentinel := planExit(plan)
			writeAuditSummary(aw, opname, true, plan, 0, "success", start)
			return code, ignoreIfJSON(emitJSON(e.stdout, opname, code, nil, data, "", nil), sentinel)
		}
		if !g.quiet {
			renderPlanHuman(e.stdout, plan, opts.all)
			fmt.Fprintf(e.stdout, "\nDry-run. Would run:\n  %s %s\n", binary, joinArgs(argv))
			fmt.Fprintln(e.stdout, "Pass --apply to execute.")
		}
		writeAuditSummary(aw, opname, true, plan, 0, "success", start)
		if len(plan.Changes) > 0 {
			return exitcode.PendingChanges, ErrPending
		}
		return exitcode.Success, nil
	}

	// Apply path: nothing to do? Still refresh the synced ancestor so future
	// runs have an accurate common ancestor (the sides are in sync now).
	if len(plan.Changes) == 0 {
		sp := stateFor(cfg.Target.Path)
		bc.remote.Generated = time.Now().UTC()
		if err := bc.remote.Save(sp.synced); err != nil {
			return exitcode.GenericError, fmt.Errorf("save synced manifest: %w", err)
		}
		writeAuditSummary(aw, opname, false, plan, 0, "success", start)
		if !g.quiet && !g.json {
			fmt.Fprintln(e.stdout, "Nothing to do.")
		}
		if g.json {
			return exitcode.Success, emitJSON(e.stdout, opname, exitcode.Success, nil, plan, "", nil)
		}
		return exitcode.Success, nil
	}

	// Execute rsync. Both the child's stderr copy and our line parser write to
	// stderr concurrently, so guard it with a single lock.
	sw := newSyncWriter(e.stderr)
	filesChanged := 0
	onLine := func(line string) {
		if ic, ok := rsyncx.ParseItemized(line); ok {
			filesChanged++
			if aw != nil {
				_ = aw.Append(audit.Record{
					Operation: opname,
					Path:      ic.Path,
					Result:    ic.ChangeType(),
				})
			}
		}
		if !g.json && !g.quiet {
			fmt.Fprintln(sw, line)
		}
	}

	if g.verbose {
		e.verbosef("rsync %s %s", binary, joinArgs(argv))
	}

	rc, runErr := rsyncx.Run(ctx, binary, argv, sw, onLine)
	if ctx.Err() != nil {
		writeAuditSummaryArgv(aw, opname, false, plan, rc, "interrupted", start, argv, filesChanged, plan.Stats.BytesTotal)
		return exitcode.ExitCode(130), fmt.Errorf("interrupted")
	}
	if runErr != nil {
		writeAuditSummaryArgv(aw, opname, false, plan, rc, "error: "+runErr.Error(), start, argv, filesChanged, 0)
		return exitcode.GenericError, runErr
	}

	warnings := []string{}
	switch rc {
	case 0:
		// success
	case 23, 24:
		warnings = append(warnings, fmt.Sprintf("rsync exited %d (partial transfer / vanished files); treated as success", rc))
	default:
		writeAuditSummaryArgv(aw, opname, false, plan, rc, fmt.Sprintf("error: rsync exit %d", rc), start, argv, filesChanged, 0)
		return exitcode.GenericError, fmt.Errorf("rsync failed with exit %d", rc)
	}

	// Post-apply state updates.
	sp := stateFor(cfg.Target.Path)
	// New synced ancestor = the remote manifest as fetched.
	bc.remote.Generated = time.Now().UTC()
	if err := bc.remote.Save(sp.synced); err != nil {
		return exitcode.GenericError, fmt.Errorf("save synced manifest: %w", err)
	}

	// Push: stage deletions for purge.
	if opts.direction == planner.DirectionPush {
		if err := stageDeletions(cfg, plan); err != nil {
			return exitcode.GenericError, err
		}
	}

	writeAuditSummaryArgv(aw, opname, false, plan, rc, "success", start, argv, filesChanged, plan.Stats.BytesTotal)

	if g.json {
		return exitcode.Success, emitJSON(e.stdout, opname, exitcode.Success, warnings, plan, "", nil)
	}
	if !g.quiet {
		for _, w := range warnings {
			fmt.Fprintf(e.stderr, "warning: %s\n", w)
		}
		fmt.Fprintf(e.stdout, "%s complete: %d files changed.\n", opname, filesChanged)
		if opts.direction == planner.DirectionPush && plan.Stats.Deletions > 0 {
			fmt.Fprintf(e.stdout, "%d deletions staged; run 'photolib purge' to execute them on %s.\n",
				plan.Stats.Deletions, cfg.Source.Host)
		}
	}
	return exitcode.Success, nil
}

// entryDiffers reports whether two entries differ in existence-agnostic content
// (type/size/mtime/link target). Mirrors the conflict package's semantics.
func entryDiffers(a, b snapshot.Entry) bool {
	if a.Type != b.Type {
		return true
	}
	if a.Type == snapshot.TypeSymlink {
		return a.LinkTarget != b.LinkTarget
	}
	if a.Type == snapshot.TypeDir {
		return false
	}
	return a.Size != b.Size || a.ModTime != b.ModTime
}

// remoteHasNewerChanges reports whether push should be blocked because the
// remote has changes relative to the synced ancestor that local lacks.
func remoteHasNewerChanges(bc *buildContext) bool {
	// Any path where remote differs from synced but local does not match remote.
	for p, re := range bc.remote.Entries {
		se, sOK := bc.synced.Entries[p]
		remoteChanged := !sOK || entryDiffers(se, re)
		if !remoteChanged {
			continue
		}
		le, lOK := bc.local.Entries[p]
		if !lOK || entryDiffers(le, re) {
			return true
		}
	}
	// Also: remote deleted something the synced ancestor had, and local still has it => remote change local lacks.
	for p := range bc.synced.Entries {
		_, rOK := bc.remote.Entries[p]
		_, lOK := bc.local.Entries[p]
		if !rOK && lOK {
			return true
		}
	}
	return false
}

func stageDeletions(cfg *config.Config, plan *planner.Plan) error {
	ps := &purge.PendingSet{
		Host:   cfg.Source.Host,
		Path:   cfg.Source.Path,
		Staged: time.Now().UTC().Format(time.RFC3339),
	}
	for _, c := range plan.Changes {
		if c.Kind == planner.ChangeDelete {
			ps.Deletions = append(ps.Deletions, purge.PendingDeletion{Path: c.Path, Size: c.Size})
		}
	}
	if len(ps.Deletions) == 0 {
		return purge.Clear(cfg.Target.Path)
	}
	return ps.Save(cfg.Target.Path)
}

func acquireWithWait(target, op string, wait time.Duration) (*lock.Lock, error) {
	lk, err := lock.Acquire(target, op)
	if err == nil {
		return lk, nil
	}
	if !lock.IsErrLocked(err) || wait <= 0 {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		lk, err = lock.Acquire(target, op)
		if err == nil {
			return lk, nil
		}
		if !lock.IsErrLocked(err) {
			return nil, err
		}
	}
	return nil, err
}

func writeAuditSummary(aw *audit.Writer, op string, dryRun bool, plan *planner.Plan, rsyncExit int, result string, start time.Time) {
	writeAuditSummaryArgv(aw, op, dryRun, plan, rsyncExit, result, start, nil, 0, 0)
}

func writeAuditSummaryArgv(aw *audit.Writer, op string, dryRun bool, plan *planner.Plan, rsyncExit int, result string, start time.Time, argv []string, filesChanged int, bytes int64) {
	if aw == nil {
		return
	}
	_ = aw.Append(audit.Record{
		Operation:        op,
		DryRun:           dryRun,
		RsyncArgv:        argv,
		RsyncExit:        rsyncExit,
		FilesChanged:     filesChanged,
		BytesTransferred: bytes,
		Plan:             plan,
		Result:           result,
		DurationMs:       time.Since(start).Milliseconds(),
	})
}

// ignoreIfJSON returns the sentinel for exit classification but drops it when
// the JSON emit already succeeded (JSON path still needs the code).
func ignoreIfJSON(emitErr error, sentinel error) error {
	if emitErr != nil {
		return emitErr
	}
	return sentinel
}

func joinArgs(argv []string) string {
	out := ""
	for i, a := range argv {
		if i > 0 {
			out += " "
		}
		out += a
	}
	return out
}
