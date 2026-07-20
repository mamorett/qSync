package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yourorg/photolib/internal/config"
	"github.com/yourorg/photolib/internal/exitcode"
)

func cmdInit(e *env) (exitcode.ExitCode, error) {
	if wantsHelp(e.args) {
		fmt.Fprint(e.stdout, helpText(commands["init"]))
		return exitcode.Success, nil
	}
	fs, g := newFlagSet("init", e)
	var host, sourcePath, targetPath string
	var force bool
	fs.StringVar(&host, "host", "", "remote ssh host")
	fs.StringVar(&sourcePath, "source-path", "", "absolute source path on remote")
	fs.StringVar(&targetPath, "target-path", "", "local library root")
	fs.BoolVar(&force, "force", false, "overwrite existing config; create target")
	if err := fs.Parse(e.args); err != nil {
		return exitcode.GenericError, e.parseErr(commands["init"], err)
	}

	cfg := config.Default()
	if host != "" {
		cfg.Source.Host = host
	}
	if sourcePath != "" {
		cfg.Source.Path = sourcePath
	}
	if targetPath != "" {
		cfg.Target.Path = targetPath
	}

	path := config.DiscoverConfigPath(g.config)
	if _, err := os.Stat(path); err == nil && !force {
		return exitcode.GenericError, fmt.Errorf("config already exists at %s; use --force to overwrite", path)
	}

	if err := cfg.Save(path); err != nil {
		return exitcode.GenericError, fmt.Errorf("write config: %w", err)
	}

	// Create target state dirs if the target exists locally, or create the
	// target itself only with --force.
	expanded := config.ExpandPath(cfg.Target.Path)
	var warnings []string
	if fi, err := os.Stat(expanded); err == nil && fi.IsDir() {
		if err := makeStateDirs(expanded); err != nil {
			warnings = append(warnings, err.Error())
		}
	} else if force {
		if err := os.MkdirAll(expanded, 0755); err != nil {
			warnings = append(warnings, "create target: "+err.Error())
		} else if err := makeStateDirs(expanded); err != nil {
			warnings = append(warnings, err.Error())
		}
	} else {
		warnings = append(warnings, "target path does not exist yet: "+expanded+" (create it or re-run with --force)")
	}

	if g.json {
		return exitcode.Success, emitJSON(e.stdout, "init", exitcode.Success, warnings,
			map[string]string{"config_path": path}, "", nil)
	}
	if !g.quiet {
		fmt.Fprintf(e.stdout, "wrote config: %s\n", path)
		for _, w := range warnings {
			fmt.Fprintf(e.stderr, "warning: %s\n", w)
		}
	}
	return exitcode.Success, nil
}

func makeStateDirs(target string) error {
	for _, sub := range []string{"state", "history", "tmp"} {
		if err := os.MkdirAll(filepath.Join(target, ".photolib", sub), 0755); err != nil {
			return fmt.Errorf("create %s: %w", sub, err)
		}
	}
	return nil
}
