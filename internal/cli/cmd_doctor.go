package cli

import (
	"fmt"

	"github.com/mamorett/PhotoLib/internal/config"
	"github.com/mamorett/PhotoLib/internal/doctor"
	"github.com/mamorett/PhotoLib/internal/exitcode"
)

func cmdDoctor(e *env) (exitcode.ExitCode, error) {
	if wantsHelp(e.args) {
		fmt.Fprint(e.stdout, helpText(commands["doctor"]))
		return exitcode.Success, nil
	}
	fs, g := newFlagSet("doctor", e)
	if err := fs.Parse(e.args); err != nil {
		return exitcode.GenericError, e.parseErr(commands["doctor"], err)
	}

	cfgPath := config.DiscoverConfigPath(g.config)
	cfg, cfgErr := config.Load(cfgPath)
	report := doctor.Run(cfgPath, cfg, cfgErr)

	code := exitcode.Success
	if report.HasHardFailure() {
		code = exitcode.GenericError
	}

	if g.json {
		return code, emitJSON(e.stdout, "doctor", code, report.Warnings, report, "", nil)
	}

	if !g.quiet {
		for _, c := range report.Checks {
			mark := "✓"
			if !c.OK {
				if c.Warning {
					mark = "!"
				} else {
					mark = "✗"
				}
			}
			fmt.Fprintf(e.stdout, "%s %-18s %s\n", mark, c.Name, c.Detail)
		}
	}
	if code != exitcode.Success {
		return code, fmt.Errorf("doctor found problems")
	}
	return code, nil
}
