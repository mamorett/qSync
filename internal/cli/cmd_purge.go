package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mamorett/PhotoLib/internal/audit"
	"github.com/mamorett/PhotoLib/internal/config"
	"github.com/mamorett/PhotoLib/internal/exitcode"
	"github.com/mamorett/PhotoLib/internal/output"
	"github.com/mamorett/PhotoLib/internal/purge"
	"github.com/mamorett/PhotoLib/internal/rsyncx"
	"github.com/mamorett/PhotoLib/internal/snapshot"
)

// stdinReader is overridable in tests.
var stdinReader = os.Stdin

func cmdPurge(e *env) (exitcode.ExitCode, error) {
	if wantsHelp(e.args) {
		fmt.Fprint(e.stdout, helpText(commands["purge"]))
		return exitcode.Success, nil
	}
	fs, g := newFlagSet("purge", e)
	var yes, delEmpty bool
	fs.BoolVar(&yes, "yes", false, "skip interactive confirmation")
	fs.BoolVar(&delEmpty, "delete-empty-dirs", true, "remove now-empty parent dirs")
	if err := fs.Parse(e.args); err != nil {
		return exitcode.GenericError, e.parseErr(commands["purge"], err)
	}

	cfg, _, err := e.loadConfig()
	if err != nil {
		return exitcode.GenericError, err
	}

	ps, err := purge.LoadPending(cfg.Target.Path)
	if err != nil {
		return exitcode.GenericError, err
	}
	n := ps.Count()
	if n == 0 {
		if g.json {
			return exitcode.Success, emitJSON(e.stdout, "purge", exitcode.Success, nil, ps, "", nil)
		}
		if !g.quiet {
			fmt.Fprintln(e.stdout, "nothing to purge")
		}
		return exitcode.Success, nil
	}

	host := ps.Host
	if host == "" {
		host = cfg.Source.Host
	}
	root := ps.Path
	if root == "" {
		root = cfg.Source.Path
	}

	if !g.quiet && !g.json {
		fmt.Fprintf(e.stdout, "%d files staged for deletion on %s:%s\n", n, host, root)
		for _, d := range ps.Deletions {
			fmt.Fprintf(e.stdout, "  %s %-40s %s\n", output.SymbolDelete, d.Path, output.HumanizeIEC(d.Size))
		}
	}

	// Confirmation.
	if !yes {
		want := purge.ConfirmPhrase(n)
		fmt.Fprintf(e.stdout, "Type '%s' to confirm: ", want)
		reader := bufio.NewReader(stdinReader)
		line, rerr := reader.ReadString('\n')
		if rerr != nil && line == "" {
			fmt.Fprintln(e.stderr, "aborted")
			return exitcode.GenericError, fmt.Errorf("aborted")
		}
		if strings.TrimSpace(line) != want {
			fmt.Fprintln(e.stderr, "aborted")
			return exitcode.GenericError, fmt.Errorf("aborted")
		}
	}

	// Execute deletions on the remote.
	paths := make([]string, 0, n)
	for _, d := range ps.Deletions {
		paths = append(paths, d.Path)
	}
	sshBin := cfg.Transport.SSH
	if sshBin == "" {
		sshBin = "ssh"
	}
	cmds, err := purge.RemoteDeleteCommands(sshBin, host, root, paths, 500)
	if err != nil {
		return exitcode.GenericError, err
	}

	start := time.Now()
	aw, _ := audit.NewWriter(cfg.Target.Path, "purge", start)
	if aw != nil {
		defer aw.Close()
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

	for _, c := range cmds {
		args := c
		if cfg.Transport.Port != 0 {
			args = append([]string{"-p", fmt.Sprintf("%d", cfg.Transport.Port)}, c...)
		}
		cmd := exec.CommandContext(ctx, sshBin, args...)
		rsyncx.ConfigurePGID(cmd)
		cmd.Stdout = e.stderr
		cmd.Stderr = e.stderr
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				writeAuditSummary(aw, "purge", false, nil, 1, "interrupted", start)
				return exitcode.ExitCode(130), fmt.Errorf("interrupted")
			}
			writeAuditSummary(aw, "purge", false, nil, 1, "error: "+err.Error(), start)
			return exitcode.GenericError, fmt.Errorf("remote delete failed: %w", err)
		}
	}

	// Update manifests: remove purged paths.
	removePurgedFromManifests(cfg, paths)

	// Clear pending file.
	if err := purge.Clear(cfg.Target.Path); err != nil {
		return exitcode.GenericError, err
	}

	if aw != nil {
		_ = aw.Append(audit.Record{Operation: "purge", FilesChanged: n, Result: "success", DurationMs: time.Since(start).Milliseconds()})
	}

	if g.json {
		return exitcode.Success, emitJSON(e.stdout, "purge", exitcode.Success, nil, map[string]int{"deleted": n}, "", nil)
	}
	if !g.quiet {
		fmt.Fprintf(e.stdout, "purged %d files on %s\n", n, host)
	}
	return exitcode.Success, nil
}

// removePurgedFromManifests strips strips purged paths from local/remote/synced
// manifests and rewrites them atomically. Best-effort: missing files are skipped.
func removePurgedFromManifests(cfg *config.Config, paths []string) {
	sp := stateFor(cfg.Target.Path)
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		set[p] = true
	}
	for _, mp := range []string{sp.local, sp.remote, sp.synced} {
		m, err := snapshot.LoadManifest(mp)
		if err != nil {
			continue
		}
		for p := range set {
			delete(m.Entries, p)
		}
		_ = m.Save(mp)
	}
}
