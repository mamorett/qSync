package rsyncx

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/yourorg/qsync/internal/config"
	"github.com/yourorg/qsync/internal/planner"
)

// itemizeFormat is the --out-format used when itemizing changes.
const itemizeFormat = "%n|%i|%b|%l"

// BuildArgs constructs the rsync argument vector (without the leading binary
// path) for the given direction. The order is deterministic.
//
// SAFETY: this function never emits any --delete* flag unless deleteMode is true.
func BuildArgs(direction planner.Direction, cfg *config.Config, itemize bool, deleteMode bool) []string {
	vfat := cfg != nil && cfg.Defaults.VFAT
	return BuildArgsVFAT(direction, cfg, itemize, deleteMode, vfat, "")
}

// BuildArgsVFAT constructs the rsync argument vector with explicit VFAT options.
func BuildArgsVFAT(direction planner.Direction, cfg *config.Config, itemize bool, deleteMode bool, vfat bool, filesFrom string) []string {
	var args []string
	if vfat {
		modWindow := 3602
		if cfg != nil && cfg.Defaults.ModifyWindow > 0 {
			modWindow = cfg.Defaults.ModifyWindow
		}
		args = []string{
			"-rltz",
			"--numeric-ids",
			"--no-perms",
			"--no-owner",
			"--no-group",
			fmt.Sprintf("--modify-window=%d", modWindow),
		}
	} else {
		args = []string{
			"-avz",
			"--numeric-ids",
			"--no-perms",
			"--chmod=ugo=rwX",
		}
		if cfg != nil && cfg.Defaults.ModifyWindow > 0 {
			args = append(args, fmt.Sprintf("--modify-window=%d", cfg.Defaults.ModifyWindow))
		}
	}

	if itemize {
		args = append(args, "--out-format="+itemizeFormat)
	}

	if deleteMode {
		args = append(args, "--delete")
	}

	// Always exclude our own state directory.
	args = append(args, "--exclude=.qsync/***")
	for _, pat := range cfg.Ignore {
		args = append(args, "--exclude="+pat)
	}

	if filesFrom != "" {
		args = append(args, "--files-from="+filesFrom)
	}

	// SSH transport.
	ssh := cfg.Transport.SSH
	if ssh == "" {
		ssh = "ssh"
	}
	if cfg.Transport.Port != 0 {
		args = append(args, "-e", fmt.Sprintf("%s -p %d", ssh, cfg.Transport.Port))
	} else {
		args = append(args, "-e", ssh)
	}

	if cfg.Rsync.BandwidthLimitKB > 0 {
		args = append(args, fmt.Sprintf("--bwlimit=%d", cfg.Rsync.BandwidthLimitKB))
	}

	args = append(args, cfg.Rsync.ExtraArgs...)

	src, dst := endpoints(direction, cfg)
	args = append(args, src, dst)
	return args
}

// endpoints returns the (source, dest) rsync path strings with mandatory
// trailing slashes on both.
func endpoints(direction planner.Direction, cfg *config.Config) (string, string) {
	remote := fmt.Sprintf("%s:%s/", cfg.Source.Host, strings.TrimRight(cfg.Source.Path, "/"))
	local := strings.TrimRight(cfg.Target.Path, "/") + "/"
	if direction == planner.DirectionPull {
		return remote, local
	}
	return local, remote
}

// Describe returns human descriptions of source and dest for a plan.
func Describe(direction planner.Direction, cfg *config.Config) (src, dst string) {
	remote := fmt.Sprintf("%s:%s", cfg.Source.Host, strings.TrimRight(cfg.Source.Path, "/"))
	local := strings.TrimRight(cfg.Target.Path, "/")
	if direction == planner.DirectionPull {
		return remote, local
	}
	return local, remote
}

// Binary returns the rsync binary path from config, defaulting to "rsync".
func Binary(cfg *config.Config) string {
	if cfg.Transport.Rsync != "" {
		return cfg.Transport.Rsync
	}
	return "rsync"
}

// ItemizedChange is one parsed line of itemized output.
type ItemizedChange struct {
	Path         string
	ItemizeFlags string
	Bytes        int64
	Length       int64
}

// ParseItemized parses one --out-format=%n|%i|%b|%l line. Lenient: returns
// ok=false for lines that don't parse, so callers can skip them.
func ParseItemized(line string) (ItemizedChange, bool) {
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return ItemizedChange{}, false
	}
	parts := strings.Split(line, "|")
	if len(parts) < 2 {
		return ItemizedChange{}, false
	}
	ic := ItemizedChange{
		Path:         parts[0],
		ItemizeFlags: parts[1],
	}
	if ic.Path == "" {
		return ItemizedChange{}, false
	}
	if len(parts) >= 3 {
		fmt.Sscan(parts[2], &ic.Bytes)
	}
	if len(parts) >= 4 {
		fmt.Sscan(parts[3], &ic.Length)
	}
	return ic, true
}

// ChangeType maps an itemize flag string to a coarse change label.
func (ic ItemizedChange) ChangeType() string {
	if len(ic.ItemizeFlags) == 0 {
		return "unknown"
	}
	switch ic.ItemizeFlags[0] {
	case '>':
		return "received"
	case '<':
		return "sent"
	case 'c':
		return "created"
	case '*':
		// e.g. *deleting — but we never delete, so treat as message.
		return "message"
	case '.':
		return "unchanged"
	default:
		return "other"
	}
}

// Run executes rsync with the given argv. In streaming (non-capture) mode the
// child's stdout is parsed line-by-line and forwarded to onLine while stderr is
// forwarded to stderrW. Returns the process exit code (0 on success) and an
// error only for exec failures other than a non-zero exit.
func Run(ctx context.Context, binary string, argv []string, stderrW io.Writer, onLine func(string)) (int, error) {
	cmd := exec.CommandContext(ctx, binary, argv...)
	configurePGID(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 1, err
	}
	cmd.Stderr = stderrW

	if err := cmd.Start(); err != nil {
		return 1, fmt.Errorf("start rsync: %w", err)
	}

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if onLine != nil {
			onLine(sc.Text())
		}
	}

	waitErr := cmd.Wait()
	if waitErr == nil {
		return 0, nil
	}
	if exitErr, ok := waitErr.(*exec.ExitError); ok {
		return exitErr.ExitCode(), nil
	}
	return 1, waitErr
}
