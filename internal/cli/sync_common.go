package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/yourorg/qsync/internal/audit"
	"github.com/yourorg/qsync/internal/config"
	"github.com/yourorg/qsync/internal/exitcode"
	"github.com/yourorg/qsync/internal/lock"
	"github.com/yourorg/qsync/internal/planner"
	"github.com/yourorg/qsync/internal/progress"
	"github.com/yourorg/qsync/internal/purge"
	"github.com/yourorg/qsync/internal/rsyncx"
	"github.com/yourorg/qsync/internal/snapshot"
)

// syncOptions configure a pull/push run.
type syncOptions struct {
	direction planner.Direction
	apply     bool
	all       bool
	waitLock  time.Duration
	delete    bool
	vfat      bool
}

// runSync executes the shared pull/push flow.
func runSync(e *env, opname string, opts syncOptions) (exitcode.ExitCode, error) {
	g := &e.globals
	cfg, _, err := e.loadConfig()
	if err != nil {
		return exitcode.GenericError, err
	}

	effectiveVFAT := opts.vfat || cfg.Defaults.VFAT

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
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			cancel()
			// If a second signal arrives, force exit immediately
			select {
			case <-sigCh:
				os.Exit(130)
			case <-time.After(3 * time.Second):
			}
		case <-ctx.Done():
		}
	}()

	start := time.Now()
	aw, _ := audit.NewWriter(cfg.Target.Path, opname, start)
	if aw != nil {
		defer aw.Close()
	}

	bc, scanErrs, err := gatherAndPlanContext(ctx, cfg, opts.direction, effectiveVFAT)
	if err != nil {
		if ctx.Err() != nil {
			return exitcode.ExitCode(130), fmt.Errorf("interrupted")
		}
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
		if violated := remoteHasNewerChanges(bc, effectiveVFAT); violated {
			msg := "DGX has newer changes; run qsync pull first"
			writeAuditSummary(aw, opname, opts.apply, plan, 0, "conflicts", start)
			if g.json {
				return exitcode.Conflicts, emitJSON(e.stdout, opname, exitcode.Conflicts, nil, plan, msg, nil)
			}
			fmt.Fprintf(e.stderr, "error: %s\n", msg)
			return exitcode.Conflicts, ErrConflicts
		}
	}

	var transferListPath string
	if opts.apply {
		var err error
		transferListPath, err = writeTransferList(cfg.Target.Path, plan.Changes)
		if err != nil {
			return exitcode.GenericError, fmt.Errorf("write transfer list: %w", err)
		}
		if transferListPath != "" {
			defer os.Remove(transferListPath)
		}
	}

	argv := rsyncx.BuildArgsVFAT(opts.direction, cfg, true, opts.delete, effectiveVFAT, transferListPath)
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

	sw := newSyncWriter(e.stderr)

	shouldDeleteLocal := opts.direction == planner.DirectionPull && opts.delete

	totalItems := 0
	for _, c := range plan.Changes {
		if c.Kind == planner.ChangeAdd || c.Kind == planner.ChangeUpdate {
			totalItems++
		} else if c.Kind == planner.ChangeDelete && shouldDeleteLocal {
			totalItems++
		}
	}

	var pb *progress.ProgressBar
	if !g.json && !g.quiet {
		opTitle := "Push"
		if opts.direction == planner.DirectionPull {
			opTitle = "Pull"
		}
		pb = progress.NewProgressBar(totalItems, opTitle)
	}

	filesChanged := 0

	// Handle local deletions for pull --apply --delete before or after transfer
	if shouldDeleteLocal {
		for _, c := range plan.Changes {
			if c.Kind == planner.ChangeDelete {
				localPath := filepath.Join(cfg.Target.Path, c.Path)
				err := os.RemoveAll(localPath)
				if err != nil && !os.IsNotExist(err) {
					if pb != nil {
						pb.UpdateError(c.Path, localPath, err.Error())
					}
				} else {
					filesChanged++
					if aw != nil {
						_ = aw.Append(audit.Record{
							Operation: opname,
							Path:      c.Path,
							Result:    "deleted",
						})
					}
					if pb != nil {
						pb.UpdateDelete(c.Path, localPath)
					}
				}
			}
		}
	}

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
			if pb != nil {
				srcPath := getSourcePath(cfg, opts.direction, ic.Path)
				pb.UpdateCopy(ic.Path, srcPath)
			}
		} else if pb != nil && strings.TrimSpace(line) != "" {
			if strings.HasPrefix(line, "rsync:") || strings.HasPrefix(line, "rsync error:") {
				pb.UpdateError("", "", line)
			} else {
				pb.LogAbove(line)
			}
		}
	}

	if g.verbose {
		e.verbosef("rsync %s %s", binary, joinArgs(argv))
	}

	var rsyncStderr io.Writer = sw
	if pb != nil {
		rsyncStderr = &progressWriter{w: sw, pb: pb}
	}

	rc, runErr := rsyncx.Run(ctx, binary, argv, rsyncStderr, onLine)
	if pb != nil {
		pb.Finish()
	}
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

	// Push: stage deletions for purge (only if not already deleted by rsync).
	if opts.direction == planner.DirectionPush && !opts.delete {
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
		if opts.direction == planner.DirectionPush && plan.Stats.Deletions > 0 && !opts.delete {
			fmt.Fprintf(e.stdout, "%d deletions staged; run 'qsync purge' to execute them on %s.\n",
				plan.Stats.Deletions, cfg.Source.Host)
		}
	}
	return exitcode.Success, nil
}

func mtimeEqual(t1, t2 int64, vfat bool) bool {
	if t1 == t2 {
		return true
	}
	diff := t1 - t2
	if diff < 0 {
		diff = -diff
	}
	if vfat {
		if diff <= 2 {
			return true
		}
		if (diff >= 3598 && diff <= 3602) || (diff >= 7198 && diff <= 7202) {
			return true
		}
	}
	return false
}

// entryDiffers reports whether two entries differ in existence-agnostic content
// (type/size/mtime/link target). Mirrors the conflict package's semantics.
func entryDiffers(a, b snapshot.Entry, vfat bool) bool {
	if a.Type != b.Type {
		return true
	}
	if a.Type == snapshot.TypeSymlink {
		return a.LinkTarget != b.LinkTarget
	}
	if a.Type == snapshot.TypeDir {
		return false
	}
	return a.Size != b.Size || !mtimeEqual(a.ModTime, b.ModTime, vfat)
}

// remoteHasNewerChanges reports whether push should be blocked because the
// remote has changes relative to the synced ancestor that local lacks.
func remoteHasNewerChanges(bc *buildContext, vfat bool) bool {
	// Any path where remote differs from synced but local does not match remote.
	for p, re := range bc.remote.Entries {
		se, sOK := bc.synced.Entries[p]
		remoteChanged := !sOK || entryDiffers(se, re, vfat)
		if !remoteChanged {
			continue
		}
		le, lOK := bc.local.Entries[p]
		if !lOK || entryDiffers(le, re, vfat) {
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

func writeTransferList(targetPath string, changes []planner.Change) (string, error) {
	var lines []string
	for _, c := range changes {
		if c.Kind == planner.ChangeAdd || c.Kind == planner.ChangeUpdate {
			lines = append(lines, c.Path)
		}
	}
	if len(lines) == 0 {
		return "", nil
	}

	tmpDir := filepath.Join(targetPath, ".qsync", "tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(tmpDir, "transfer-*.txt")
	if err != nil {
		return "", err
	}
	defer f.Close()

	content := strings.Join(lines, "\n") + "\n"
	if _, err := f.WriteString(content); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func getSourcePath(cfg *config.Config, direction planner.Direction, relPath string) string {
	if direction == planner.DirectionPush {
		return filepath.Join(cfg.Target.Path, relPath)
	}
	remPath := strings.TrimRight(cfg.Source.Path, "/") + "/" + relPath
	if cfg.Source.Host != "" {
		return cfg.Source.Host + ":" + remPath
	}
	return remPath
}

type progressWriter struct {
	w  io.Writer
	pb *progress.ProgressBar
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	if pw.pb != nil {
		pw.pb.LogAbove(strings.TrimRight(string(p), "\r\n"))
		return len(p), nil
	}
	return pw.w.Write(p)
}

