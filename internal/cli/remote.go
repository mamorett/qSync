package cli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/mamorett/qsync/internal/config"
	"github.com/mamorett/qsync/internal/rsyncx"
	"github.com/mamorett/qsync/internal/snapshot"
)

// fetchRemoteManifest runs `ssh <host> qsync scan --root <path>` and parses
// the JSONL manifest from stdout. With checksum=true it requests hashes.
func fetchRemoteManifest(cfg *config.Config, checksum bool) (*snapshot.Manifest, error) {
	return fetchRemoteManifestContext(context.Background(), cfg, checksum)
}

func fetchRemoteManifestContext(ctx context.Context, cfg *config.Config, checksum bool) (*snapshot.Manifest, error) {
	sshBin := cfg.Transport.SSH
	if sshBin == "" {
		sshBin = "ssh"
	}
	args := []string{}
	if cfg.Transport.Port != 0 {
		args = append(args, "-p", fmt.Sprintf("%d", cfg.Transport.Port))
	}
	remoteCmd := "qsync scan --root " + shellArg(cfg.Source.Path)
	if checksum {
		remoteCmd += " --checksum"
	}
	args = append(args, cfg.Source.Host, remoteCmd)

	cmd := exec.CommandContext(ctx, sshBin, args...)
	rsyncx.ConfigurePGID(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("interrupted")
		}
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(msg, "not found") || strings.Contains(msg, "command not found") {
			return nil, fmt.Errorf("qsync binary not found on %s; install it there first (see README \"Remote Setup\")", cfg.Source.Host)
		}
		if msg != "" {
			return nil, fmt.Errorf("remote scan failed: %s", firstLine(msg))
		}
		return nil, fmt.Errorf("remote scan failed: %w", err)
	}
	m, err := snapshot.Read(&stdout)
	if err != nil {
		return nil, fmt.Errorf("parse remote manifest: %w", err)
	}
	return m, nil
}

func shellArg(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$`*?[]{}();&|<>#~!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
