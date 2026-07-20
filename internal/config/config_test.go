package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCfg(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_DryRunDefaultsTrueWhenAbsent(t *testing.T) {
	p := writeCfg(t, `
source:
  host: dgx
  path: /photos
target:
  path: /tmp/pics
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Defaults.DryRun {
		t.Fatal("DryRun should default to true when absent")
	}
}

func TestLoad_DryRunFalseHonored(t *testing.T) {
	p := writeCfg(t, `
source:
  host: dgx
  path: /photos
target:
  path: /tmp/pics
defaults:
  dry_run: false
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.DryRun {
		t.Fatal("DryRun false should be honored")
	}
}

func TestLoad_UnknownKeyRejected(t *testing.T) {
	p := writeCfg(t, `
source:
  host: dgx
  path: /photos
target:
  path: /tmp/pics
bogus_key: 1
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error on unknown key")
	}
}

func TestValidate_AggregatesErrors(t *testing.T) {
	c := &Config{} // everything missing
	err := c.Validate()
	if err == nil {
		t.Fatal("expected validation errors")
	}
	msg := err.Error()
	for _, want := range []string{"source.host", "source.path", "target.path"} {
		if !contains(msg, want) {
			t.Errorf("missing %q in aggregated error: %s", want, msg)
		}
	}
}

func TestValidate_TargetMustBeAbsolute(t *testing.T) {
	c := &Config{
		Source: SourceConfig{Host: "h", Path: "/p"},
		Target: TargetConfig{Path: "relative/path"},
	}
	if err := c.Validate(); err == nil || !contains(err.Error(), "absolute") {
		t.Fatalf("expected absolute-path error, got %v", err)
	}
}

func TestDefault(t *testing.T) {
	c := Default()
	if c.Source.Host != "dgx" || c.Source.Path != "/photos" {
		t.Errorf("unexpected default source: %+v", c.Source)
	}
	if c.Target.Path != "~/Pictures" {
		t.Errorf("unexpected default target: %s", c.Target.Path)
	}
	if !c.Defaults.DryRun {
		t.Error("default DryRun should be true")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "config.yaml")
	c := Default()
	c.Target.Path = "/tmp/lib"
	if err := c.Save(p); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("config perms = %o, want 0600", fi.Mode().Perm())
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source.Host != c.Source.Host {
		t.Errorf("roundtrip host mismatch")
	}
}

func TestDiscoverConfigPath(t *testing.T) {
	if got := DiscoverConfigPath("/explicit"); got != "/explicit" {
		t.Errorf("flag should win, got %s", got)
	}
	t.Setenv("PHOTOLIB_CONFIG", "/from/env")
	if got := DiscoverConfigPath(""); got != "/from/env" {
		t.Errorf("env should win when no flag, got %s", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
