package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/yourorg/photolib/internal/config"
	"github.com/yourorg/photolib/internal/exitcode"
)

// Version is set at build time via -ldflags "-X ...cli.Version=...".
var Version = "dev"

// Sentinel errors mapped to exit codes by Run.
var (
	ErrNotImplemented = errors.New("not implemented")
	ErrConflicts      = errors.New("conflicts detected")
	ErrVerifyFailed   = errors.New("verification failed")
	ErrPending        = errors.New("changes pending")
	ErrLockActive     = errors.New("lock active")
	// errUsage signals a flag/usage problem (exit 1, message already printed).
	errUsage = errors.New("usage error")
)

// globalFlags holds flags available on every subcommand.
type globalFlags struct {
	config  string
	json    bool
	quiet   bool
	verbose bool
}

// env carries shared I/O and parsed globals into command handlers.
type env struct {
	stdout  io.Writer
	stderr  io.Writer
	globals globalFlags
	args    []string // remaining args after global flags removed
}

// commandFunc runs a subcommand and returns an exit code plus an error used only
// for classification by Run.
type commandFunc func(e *env) (exitcode.ExitCode, error)

// command describes one subcommand.
type command struct {
	name    string
	group   string // "Setup" | "Inspect" | "Sync" | ""
	summary string
	hidden  bool
	run     commandFunc
}

// registry of commands, populated in init below.
var commands = map[string]*command{}

func register(c *command) { commands[c.name] = c }

func init() {
	register(&command{name: "init", group: "Setup", summary: "create a config file and target state dirs", run: cmdInit})
	register(&command{name: "doctor", group: "Setup", summary: "run environment checks", run: cmdDoctor})
	register(&command{name: "status", group: "Inspect", summary: "show local changes since last sync", run: cmdStatus})
	register(&command{name: "plan", group: "Inspect", summary: "compute pending changes (no mutation)", run: cmdPlan})
	register(&command{name: "verify", group: "Inspect", summary: "verify integrity against the remote", run: cmdVerify})
	register(&command{name: "pull", group: "Sync", summary: "pull changes from the DGX (dry-run unless --apply)", run: cmdPull})
	register(&command{name: "push", group: "Sync", summary: "push changes to the DGX (dry-run unless --apply)", run: cmdPush})
	register(&command{name: "purge", group: "Sync", summary: "execute staged deletions (requires confirmation)", run: cmdPurge})
	register(&command{name: "scan", summary: "scan a root and emit a manifest (internal)", hidden: true, run: cmdScan})
}

// Run parses global flags, dispatches to a subcommand, and returns an exit code.
// It never calls os.Exit.
func Run(args []string, stdout, stderr io.Writer) exitcode.ExitCode {
	if len(args) == 0 {
		printTopUsage(stderr)
		return exitcode.GenericError
	}

	switch args[0] {
	case "-h", "--help", "help":
		return runHelp(args[1:], stdout, stderr)
	case "-v", "version", "--version":
		fmt.Fprintf(stdout, "%s\n", versionString())
		return exitcode.Success
	}

	name := args[0]
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(stderr, "unknown command: %s\n\n", name)
		printTopUsage(stderr)
		return exitcode.GenericError
	}

	e := &env{stdout: stdout, stderr: stderr, args: args[1:]}
	code, err := cmd.run(e)
	if err != nil {
		return classify(e, cmd, code, err)
	}
	return code
}

// classify maps sentinel errors to exit codes.
func classify(e *env, cmd *command, code exitcode.ExitCode, err error) exitcode.ExitCode {
	switch {
	case errors.Is(err, errUsage):
		return exitcode.GenericError
	case errors.Is(err, ErrConflicts):
		return exitcode.Conflicts
	case errors.Is(err, ErrLockActive):
		return exitcode.LockActive
	case errors.Is(err, ErrVerifyFailed):
		return exitcode.VerifyFailed
	case errors.Is(err, ErrPending):
		return exitcode.PendingChanges
	case errors.Is(err, ErrNotImplemented):
		fmt.Fprintf(e.stderr, "error: %s: not implemented\n", cmd.name)
		return exitcode.GenericError
	default:
		fmt.Fprintf(e.stderr, "error: %v\n", err)
		if code != exitcode.Success {
			return code
		}
		return exitcode.GenericError
	}
}

// newFlagSet builds a FlagSet with global flags wired and error output silenced
// (we render errors ourselves).
func newFlagSet(name string, e *env) (*flag.FlagSet, *globalFlags) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	g := &e.globals
	fs.StringVar(&g.config, "config", "", "config file path")
	fs.BoolVar(&g.json, "json", false, "machine-readable JSON output")
	fs.BoolVar(&g.quiet, "quiet", false, "suppress non-error output")
	fs.BoolVar(&g.quiet, "q", false, "suppress non-error output")
	fs.BoolVar(&g.verbose, "verbose", false, "extra diagnostics on stderr")
	fs.BoolVar(&g.verbose, "v", false, "extra diagnostics on stderr")
	return fs, g
}

// loadConfig resolves and loads the config for a command.
func (e *env) loadConfig() (*config.Config, string, error) {
	path := config.DiscoverConfigPath(e.globals.config)
	cfg, err := config.Load(path)
	return cfg, path, err
}

func (e *env) verbosef(format string, a ...any) {
	if e.globals.verbose {
		fmt.Fprintf(e.stderr, "# "+format+"\n", a...)
	}
}

func versionString() string {
	return fmt.Sprintf("photolib %s (%s)", Version, goPlatform())
}

// printTopUsage renders top-level help with grouped commands.
func printTopUsage(w io.Writer) {
	fmt.Fprintln(w, "photolib — safe, deterministic photo library sync (rsync/ssh wrapper)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage: photolib <command> [flags]")
	fmt.Fprintln(w)
	groups := []string{"Setup", "Inspect", "Sync"}
	for _, grp := range groups {
		fmt.Fprintf(w, "%s:\n", grp)
		var names []string
		for n, c := range commands {
			if c.group == grp && !c.hidden {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintf(w, "  %-9s %s\n", n, commands[n].summary)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "Other:")
	fmt.Fprintln(w, "  version   print version")
	fmt.Fprintln(w, "  help      show help for a command")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Exit codes: 0 ok, 1 error, 2 conflicts, 3 lock held, 4 verify failed, 5 pending changes.")
	fmt.Fprintln(w, "Dry-run is the default for pull/push; pass --apply to mutate.")
}

func runHelp(args []string, stdout, stderr io.Writer) exitcode.ExitCode {
	if len(args) == 0 {
		printTopUsage(stdout)
		return exitcode.Success
	}
	name := args[0]
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(stderr, "unknown command: %s\n", name)
		return exitcode.GenericError
	}
	fmt.Fprint(stdout, helpText(cmd))
	return exitcode.Success
}

// isHelpRequest reports whether args request help for a command.
func isHelpRequest(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

// parseErr renders a flag parse error and returns errUsage.
func (e *env) parseErr(cmd *command, err error) error {
	fmt.Fprintf(e.stderr, "error: %v\n\n", err)
	fmt.Fprint(e.stderr, helpText(cmd))
	return errUsage
}

// firstNonFlag returns true if s looks like it wants help handling.
func wantsHelp(args []string) bool {
	return isHelpRequest(args) || (len(args) > 0 && strings.TrimSpace(args[0]) == "help")
}
