package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/yourorg/qsync/internal/exitcode"
	"github.com/yourorg/qsync/internal/lock"
	"github.com/yourorg/qsync/internal/planner"
)

func cmdPlan(e *env) (exitcode.ExitCode, error) {
	if wantsHelp(e.args) {
		fmt.Fprint(e.stdout, helpText(commands["plan"]))
		return exitcode.Success, nil
	}
	fs, g := newFlagSet("plan", e)
	var dir string
	var all, vfat bool
	fs.StringVar(&dir, "direction", "pull", "pull|push")
	fs.BoolVar(&all, "all", false, "list all changes (not just first 50)")
	fs.BoolVar(&vfat, "vfat", false, "enable VFAT/exFAT mode (2-sec mtime window, suppress perms)")
	if err := fs.Parse(e.args); err != nil {
		return exitcode.GenericError, e.parseErr(commands["plan"], err)
	}

	direction, err := parseDirection(dir)
	if err != nil {
		return exitcode.GenericError, err
	}

	cfg, _, err := e.loadConfig()
	if err != nil {
		return exitcode.GenericError, err
	}

	effectiveVFAT := vfat || cfg.Defaults.VFAT

	// Refuse if a lock is held (plans during active sync are unreliable).
	if held, info, _ := lock.IsLocked(cfg.Target.Path); held {
		if g.json {
			return exitcode.LockActive, emitJSON(e.stdout, "plan", exitcode.LockActive, nil, nil,
				fmt.Sprintf("lock held by pid %d", info.PID), nil)
		}
		return exitcode.LockActive, ErrLockActive
	}

	sigCh := make(chan os.Signal, 2)
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

	bc, scanErrs, err := gatherAndPlanContext(ctx, cfg, direction, effectiveVFAT)
	if err != nil {
		if ctx.Err() != nil {
			return exitcode.ExitCode(130), fmt.Errorf("interrupted")
		}
		return exitcode.GenericError, err
	}
	for _, se := range scanErrs {
		e.verbosef("scan warning: %s", se.Error())
	}

	code, sentinel := planExit(bc.plan)

	if g.json {
		errMsg := ""
		var conflicts []planner.Conflict
		if bc.plan.Stats.Conflicts > 0 {
			errMsg = "conflicts detected"
			conflicts = bc.plan.Conflicts
		}
		if err := emitJSON(e.stdout, "plan", code, nil, bc.plan, errMsg, conflicts); err != nil {
			return exitcode.GenericError, err
		}
		return code, sentinel
	}

	if !g.quiet {
		renderPlanHuman(e.stdout, bc.plan, all)
	}
	return code, sentinel
}

func parseDirection(s string) (planner.Direction, error) {
	switch s {
	case "pull", "":
		return planner.DirectionPull, nil
	case "push":
		return planner.DirectionPush, nil
	default:
		return planner.DirectionPull, fmt.Errorf("invalid direction %q; want pull or push", s)
	}
}
