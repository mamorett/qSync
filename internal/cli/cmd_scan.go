package cli

import (
	"fmt"
	"os"

	"github.com/yourorg/photolib/internal/config"
	"github.com/yourorg/photolib/internal/exitcode"
	"github.com/yourorg/photolib/internal/snapshot"
)

// cmdScan is a hidden subcommand: scans a root and writes a JSONL manifest to
// stdout. Used by pull/plan over SSH and by integration tests.
func cmdScan(e *env) (exitcode.ExitCode, error) {
	if wantsHelp(e.args) {
		fmt.Fprint(e.stdout, helpText(commands["scan"]))
		return exitcode.Success, nil
	}
	fs, _ := newFlagSet("scan", e)
	var root string
	var checksum bool
	fs.StringVar(&root, "root", "", "root directory to scan")
	fs.BoolVar(&checksum, "checksum", false, "include SHA-256 hashes")
	if err := fs.Parse(e.args); err != nil {
		return exitcode.GenericError, e.parseErr(commands["scan"], err)
	}

	// Integration-test hook: override root via env.
	if fake := os.Getenv("PHOTOLIB_FAKE_ROOT"); fake != "" {
		root = fake
	}
	if root == "" {
		return exitcode.GenericError, fmt.Errorf("scan: --root is required")
	}

	// Load ignore patterns from config if available (best-effort).
	var ignore []string
	if cfg, err := config.Load(config.DiscoverConfigPath(e.globals.config)); err == nil {
		ignore = cfg.Ignore
	}

	res, err := snapshot.Scan(root, ignore)
	if err != nil {
		return exitcode.GenericError, err
	}
	if checksum {
		if err := snapshot.AddHashes(res.Manifest, root); err != nil {
			return exitcode.GenericError, err
		}
	}
	if err := res.Manifest.Write(e.stdout); err != nil {
		return exitcode.GenericError, err
	}
	// Scan errors are reported to stderr but do not fail the scan itself.
	for _, se := range res.Errors {
		fmt.Fprintf(e.stderr, "# scan warning: %s\n", se.Error())
	}
	return exitcode.Success, nil
}
