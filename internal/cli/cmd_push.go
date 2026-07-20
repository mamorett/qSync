package cli

import (
	"fmt"
	"time"

	"github.com/yourorg/qsync/internal/exitcode"
	"github.com/yourorg/qsync/internal/planner"
)

func cmdPush(e *env) (exitcode.ExitCode, error) {
	if wantsHelp(e.args) {
		fmt.Fprint(e.stdout, helpText(commands["push"]))
		return exitcode.Success, nil
	}
	fs, _ := newFlagSet("push", e)
	var apply, all bool
	var waitLock time.Duration
	fs.BoolVar(&apply, "apply", false, "actually transfer (default is dry-run)")
	fs.BoolVar(&all, "all", false, "list all changes")
	fs.DurationVar(&waitLock, "wait-lock", 0, "wait up to DUR for the lock")
	if err := fs.Parse(e.args); err != nil {
		return exitcode.GenericError, e.parseErr(commands["push"], err)
	}
	return runSync(e, "push", syncOptions{
		direction: planner.DirectionPush,
		apply:     apply,
		all:       all,
		waitLock:  waitLock,
	})
}
