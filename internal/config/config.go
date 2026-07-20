package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the root of config.yaml.
type Config struct {
	Source    SourceConfig    `yaml:"source"`
	Target    TargetConfig    `yaml:"target"`
	Transport TransportConfig `yaml:"transport"`
	Defaults  DefaultsConfig  `yaml:"defaults"`
	Ignore    []string        `yaml:"ignore,omitempty"`
	Rsync     RsyncConfig     `yaml:"rsync,omitempty"`
}

type SourceConfig struct {
	Host string `yaml:"host"`
	Path string `yaml:"path"`
}

type TargetConfig struct {
	Path string `yaml:"path"`
}

type TransportConfig struct {
	SSH   string `yaml:"ssh"`
	Rsync string `yaml:"rsync"`
	Port  int    `yaml:"port,omitempty"`
}

// DefaultsConfig holds default behavior toggles. DryRun defaults to true when
// absent from the YAML (implemented via a *bool that we normalize in Load).
type DefaultsConfig struct {
	DryRun bool `yaml:"dry_run"`
}

// defaultsRaw mirrors DefaultsConfig but keeps DryRun optional so we can detect
// absence and default it to true.
type defaultsRaw struct {
	DryRun *bool `yaml:"dry_run"`
}

type RsyncConfig struct {
	ExtraArgs        []string `yaml:"extra_args,omitempty"`
	BandwidthLimitKB int      `yaml:"bwlimit_kb,omitempty"`
}

// configRaw is the wire form used for decoding so that DryRun absence is
// detectable.
type configRaw struct {
	Source    SourceConfig    `yaml:"source"`
	Target    TargetConfig    `yaml:"target"`
	Transport TransportConfig `yaml:"transport"`
	Defaults  defaultsRaw     `yaml:"defaults"`
	Ignore    []string        `yaml:"ignore,omitempty"`
	Rsync     RsyncConfig     `yaml:"rsync,omitempty"`
}

// Load reads YAML from path, expands ~ in paths, applies defaults, and
// validates. Unknown YAML keys are rejected.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var raw configRaw
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	cfg := &Config{
		Source:    raw.Source,
		Target:    raw.Target,
		Transport: raw.Transport,
		Ignore:    raw.Ignore,
		Rsync:     raw.Rsync,
	}
	// DryRun defaults to true when absent.
	if raw.Defaults.DryRun == nil {
		cfg.Defaults.DryRun = true
	} else {
		cfg.Defaults.DryRun = *raw.Defaults.DryRun
	}

	cfg.Target.Path = expandPath(cfg.Target.Path)
	cfg.Transport.SSH = expandPath(cfg.Transport.SSH)
	cfg.Transport.Rsync = expandPath(cfg.Transport.Rsync)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// expandPath expands a leading ~ to the user's home directory.
func expandPath(p string) string {
	if p == "" || !strings.HasPrefix(p, "~") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

// Validate returns a single error aggregating all problems.
func (c *Config) Validate() error {
	var errs []error

	if c.Source.Host == "" {
		errs = append(errs, errors.New("source.host is required"))
	}
	if c.Source.Path == "" {
		errs = append(errs, errors.New("source.path is required"))
	}
	if c.Target.Path == "" {
		errs = append(errs, errors.New("target.path is required"))
	} else if !filepath.IsAbs(c.Target.Path) {
		errs = append(errs, fmt.Errorf("target.path must be absolute after expansion: %s", c.Target.Path))
	}

	if err := checkBinary("ssh", c.Transport.SSH); err != nil {
		errs = append(errs, err)
	}
	if err := checkBinary("rsync", c.Transport.Rsync); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// checkBinary verifies a transport binary is executable. If the configured
// value is empty, it resolves the default name via exec.LookPath.
func checkBinary(name, configured string) error {
	if configured == "" {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("%s binary not found in PATH; set transport.%s", name, name)
		}
		return nil
	}
	if filepath.IsAbs(configured) {
		fi, err := os.Stat(configured)
		if err != nil {
			return fmt.Errorf("%s binary not accessible: %s", name, configured)
		}
		if fi.Mode()&0111 == 0 {
			return fmt.Errorf("%s binary not executable: %s", name, configured)
		}
		return nil
	}
	if _, err := exec.LookPath(configured); err != nil {
		return fmt.Errorf("%s binary not found in PATH: %s", name, configured)
	}
	return nil
}

// Default returns a config with placeholder values.
func Default() *Config {
	return &Config{
		Source: SourceConfig{
			Host: "dgx",
			Path: "/photos",
		},
		Target: TargetConfig{
			Path: "~/Pictures",
		},
		Transport: TransportConfig{},
		Defaults:  DefaultsConfig{DryRun: true},
		Rsync:     RsyncConfig{},
	}
}

// Save writes the config with 0600 perms, creating parent dirs with 0755.
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	var sb strings.Builder
	enc := yaml.NewEncoder(&sb)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return err
	}
	enc.Close()
	return os.WriteFile(path, []byte(sb.String()), 0600)
}

// DiscoverConfigPath resolves the config path: flag → env → user config dir.
func DiscoverConfigPath(flagPath string) string {
	if flagPath != "" {
		return flagPath
	}
	if p := os.Getenv("QSYNC_CONFIG"); p != "" {
		return p
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "qsync", "config.yaml")
	}
	return filepath.Join(configDir, "qsync", "config.yaml")
}

// ExpandPath is exported for callers that need ~ expansion (e.g. init).
func ExpandPath(p string) string { return expandPath(p) }
