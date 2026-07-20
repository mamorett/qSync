package cli

import (
	"fmt"
	"time"

	"github.com/yourorg/qsync/internal/exitcode"
	"github.com/yourorg/qsync/internal/planner"
)

func cmdPull(e *env) (exitcode.ExitCode, error) {
	if wantsHelp(e.args) {
		fmt.Fprint(e.stdout, helpText(commands["pull"]))
		return exitcode.Success, nil
	}
	fs, _ := newFlagSet("pull", e)
	var apply, all, delete bool
	var waitLock time.Duration
	fs.BoolVar(&apply, "apply", false, "actually transfer (default is dry-run)")
	fs.BoolVar(&all, "all", false, "list all changes")
	fs.BoolVar(&delete, "delete", false, "delete on target what is no longer on source")
	fs.DurationVar(&waitLock, "wait-lock", 0, "wait up to DUR for the lock")
	if err := fs.Parse(e.args); err != nil {
		return exitcode.GenericError, e.parseErr(commands["pull"], err)
	}
	return runSync(e, "pull", syncOptions{
		direction: planner.DirectionPull,
		apply:     apply,
		all:       all,
		waitLock:  waitLock,
		delete:    delete,
	})
}
