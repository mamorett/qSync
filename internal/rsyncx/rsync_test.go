package rsyncx

import (
	"strings"
	"testing"

	"github.com/yourorg/qsync/internal/config"
	"github.com/yourorg/qsync/internal/planner"
)

func testCfg() *config.Config {
	return &config.Config{
		Source:    config.SourceConfig{Host: "user@dgx", Path: "/photos"},
		Target:    config.TargetConfig{Path: "/home/me/Pictures"},
		Transport: config.TransportConfig{SSH: "ssh", Rsync: "rsync"},
	}
}

// SAFETY: do not remove. Ensures no --delete* flag is ever constructed.
func TestNoDeleteFlag(t *testing.T) {
	forbidden := []string{
		"--delete", "--delete-before", "--delete-during",
		"--delete-after", "--delete-excluded",
	}
	for _, dir := range []planner.Direction{planner.DirectionPull, planner.DirectionPush} {
		for _, itemize := range []bool{true, false} {
			args := BuildArgs(dir, testCfg(), itemize, false)
			joined := strings.Join(args, " ")
			for _, f := range forbidden {
				for _, a := range args {
					if a == f {
						t.Fatalf("forbidden flag %q present in argv: %s", f, joined)
					}
				}
			}
		}
	}
}

func TestDeleteFlag(t *testing.T) {
	for _, dir := range []planner.Direction{planner.DirectionPull, planner.DirectionPush} {
		args := BuildArgs(dir, testCfg(), false, true)
		hasDelete := false
		for _, a := range args {
			if a == "--delete" {
				hasDelete = true
				break
			}
		}
		if !hasDelete {
			t.Fatalf("expected --delete flag, got %v", args)
		}
	}
}

func TestBuildArgsTrailingSlashes(t *testing.T) {
	args := BuildArgs(planner.DirectionPull, testCfg(), false, false)
	src, dst := args[len(args)-2], args[len(args)-1]
	if !strings.HasSuffix(src, "/") || !strings.HasSuffix(dst, "/") {
		t.Fatalf("expected trailing slashes on both endpoints, got %q %q", src, dst)
	}
	if src != "user@dgx:/photos/" {
		t.Errorf("pull source = %q", src)
	}
	if dst != "/home/me/Pictures/" {
		t.Errorf("pull dest = %q", dst)
	}
}

func TestBuildArgsPushReversed(t *testing.T) {
	args := BuildArgs(planner.DirectionPush, testCfg(), false, false)
	src, dst := args[len(args)-2], args[len(args)-1]
	if src != "/home/me/Pictures/" || dst != "user@dgx:/photos/" {
		t.Fatalf("push endpoints wrong: %q -> %q", src, dst)
	}
}

func TestBuildArgsExcludesQsync(t *testing.T) {
	args := BuildArgs(planner.DirectionPull, testCfg(), false, false)
	found := false
	for _, a := range args {
		if a == "--exclude=.qsync/***" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected --exclude=.qsync/***")
	}
}

func TestBuildArgsPortAndBwlimit(t *testing.T) {
	cfg := testCfg()
	cfg.Transport.Port = 2222
	cfg.Rsync.BandwidthLimitKB = 1000
	args := BuildArgs(planner.DirectionPull, cfg, false, false)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "ssh -p 2222") {
		t.Errorf("missing port in -e arg: %s", joined)
	}
	if !strings.Contains(joined, "--bwlimit=1000") {
		t.Errorf("missing bwlimit: %s", joined)
	}
}

func TestParseItemized(t *testing.T) {
	cases := []struct {
		line     string
		wantPath string
		wantOK   bool
	}{
		{"2024/05/a.jpg|>f+++++++++|58|14", "2024/05/a.jpg", true},
		{"2024/|cd+++++++++|0|4096", "2024/", true},
		{"", "", false},
		{"garbage", "", false},
	}
	for _, c := range cases {
		ic, ok := ParseItemized(c.line)
		if ok != c.wantOK {
			t.Errorf("ParseItemized(%q) ok=%v want %v", c.line, ok, c.wantOK)
			continue
		}
		if ok && ic.Path != c.wantPath {
			t.Errorf("ParseItemized(%q) path=%q want %q", c.line, ic.Path, c.wantPath)
		}
	}
}

func TestBuildArgsVFAT(t *testing.T) {
	args := BuildArgsVFAT(planner.DirectionPull, testCfg(), false, false, true, "")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-rltz") {
		t.Errorf("expected -rltz in vfat args, got %s", joined)
	}
	if !strings.Contains(joined, "--modify-window=3602") {
		t.Errorf("expected --modify-window=3602 in vfat args, got %s", joined)
	}
	if !strings.Contains(joined, "--no-owner") || !strings.Contains(joined, "--no-group") {
		t.Errorf("expected permission suppression in vfat args, got %s", joined)
	}
}
