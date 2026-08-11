package cli

import (
	"fmt"

	"github.com/mamorett/qsync/internal/exitcode"
	"github.com/mamorett/qsync/internal/lock"
	"github.com/mamorett/qsync/internal/snapshot"
	"github.com/mamorett/qsync/internal/verify"
)

func cmdVerify(e *env) (exitcode.ExitCode, error) {
	if wantsHelp(e.args) {
		fmt.Fprint(e.stdout, helpText(commands["verify"]))
		return exitcode.Success, nil
	}
	fs, g := newFlagSet("verify", e)
	var checksum, all bool
	fs.BoolVar(&checksum, "checksum", false, "compare SHA-256 hashes")
	fs.BoolVar(&all, "all", false, "list all mismatches")
	if err := fs.Parse(e.args); err != nil {
		return exitcode.GenericError, e.parseErr(commands["verify"], err)
	}

	cfg, _, err := e.loadConfig()
	if err != nil {
		return exitcode.GenericError, err
	}

	if held, _, _ := lock.IsLocked(cfg.Target.Path); held {
		if g.json {
			return exitcode.LockActive, emitJSON(e.stdout, "verify", exitcode.LockActive, nil, nil, "lock held", nil)
		}
		return exitcode.LockActive, ErrLockActive
	}

	res, err := snapshot.Scan(cfg.Target.Path, cfg.Ignore)
	if err != nil {
		return exitcode.GenericError, err
	}
	local := res.Manifest
	if checksum {
		if err := snapshot.AddHashes(local, cfg.Target.Path); err != nil {
			return exitcode.GenericError, err
		}
	}

	remote, err := fetchRemoteManifest(cfg, checksum)
	if err != nil {
		return exitcode.GenericError, err
	}

	result, err := verify.Verify(local, remote, checksum, cfg.Target.Path)
	if err != nil {
		return exitcode.GenericError, err
	}

	code := exitcode.Success
	if !result.OK() {
		code = exitcode.VerifyFailed
	}

	if g.json {
		errMsg := ""
		if !result.OK() {
			errMsg = "verification failed"
		}
		if err := emitJSON(e.stdout, "verify", code, nil, result, errMsg, nil); err != nil {
			return exitcode.GenericError, err
		}
	} else if !g.quiet {
		fmt.Fprintf(e.stdout, "Checked %d files.\n", result.Checked)
		limit := 50
		for i, m := range result.Mismatches {
			if !all && i >= limit {
				fmt.Fprintf(e.stdout, "  ... (%d more; use --all)\n", len(result.Mismatches)-i)
				break
			}
			fmt.Fprintf(e.stdout, "  ! %-40s %s (expect %s, got %s)\n", m.Path, m.Kind, m.Expect, m.Got)
		}
		if result.OK() {
			fmt.Fprintln(e.stdout, "No mismatches.")
		} else {
			fmt.Fprintf(e.stdout, "%d mismatches.\n", len(result.Mismatches))
		}
	}

	if !result.OK() {
		return code, ErrVerifyFailed
	}
	return code, nil
}
