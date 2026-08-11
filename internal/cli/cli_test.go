package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mamorett/PhotoLib/internal/exitcode"
)

// buildTestBinary compiles the qsync binary once for integration tests that
// need a real `qsync scan` over the fake-ssh harness.
func buildTestBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "qsync")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/mamorett/PhotoLib/cmd/qsync")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build test binary: %v\n%s", err, out)
	}
	return bin
}

// writeFakeSSH writes a shell script emulating `ssh host <cmd>` where scan
// requests are served from remoteRoot via the built binary.
func writeFakeSSH(t *testing.T, dir, bin, remoteRoot string) string {
	t.Helper()
	path := filepath.Join(dir, "fakessh")
	script := fmt.Sprintf(`#!/bin/bash
host="$1"; shift
cmd="$*"
if echo "$cmd" | grep -q "qsync scan"; then
  QSYNC_FAKE_ROOT=%q %q scan --root /ignored
  exit 0
fi
if [ "$1" = "rsync" ]; then exec "$@"; fi
exec /bin/sh -c "$cmd"
`, remoteRoot, bin)
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeConfig(t *testing.T, dir, remoteRoot, target, fakeSSH string) string {
	t.Helper()
	rsync, err := exec.LookPath("rsync")
	if err != nil {
		t.Skip("rsync not available")
	}
	cfg := fmt.Sprintf(`source:
  host: fakehost
  path: %s
target:
  path: %s
transport:
  ssh: %s
  rsync: %s
`, remoteRoot, target, fakeSSH, rsync)
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func mkfile(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func run(args ...string) (exitcode.ExitCode, string, string) {
	var out, errb bytes.Buffer
	code := Run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestRun_Help(t *testing.T) {
	code, out, _ := run("help")
	if code != exitcode.Success {
		t.Fatalf("help exit = %d", code)
	}
	for _, want := range []string{"Setup:", "Inspect:", "Sync:", "pull", "purge"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q", want)
		}
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	code, _, _ := run("frobnicate")
	if code != exitcode.GenericError {
		t.Fatalf("unknown command exit = %d, want 1", code)
	}
}

func TestRun_Version(t *testing.T) {
	code, out, _ := run("version")
	if code != exitcode.Success || !strings.Contains(out, "qsync") {
		t.Fatalf("version bad: code=%d out=%q", code, out)
	}
}

func TestPlanExit5(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := buildTestBinary(t)
	work := t.TempDir()
	remote := filepath.Join(work, "remote")
	local := filepath.Join(work, "local")
	os.MkdirAll(local, 0755)
	mkfile(t, filepath.Join(remote, "2024/a.jpg"), "hello")

	fakeSSH := writeFakeSSH(t, work, bin, remote)
	cfgPath := writeConfig(t, work, remote, local, fakeSSH)

	code, out, errb := run("plan", "--config", cfgPath)
	if code != exitcode.PendingChanges {
		t.Fatalf("plan exit = %d, want 5\nstdout=%s\nstderr=%s", code, out, errb)
	}
	if !strings.Contains(out, "a.jpg") {
		t.Errorf("plan output missing file: %s", out)
	}
}

func TestPullDryRunLeavesTreeUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := buildTestBinary(t)
	work := t.TempDir()
	remote := filepath.Join(work, "remote")
	local := filepath.Join(work, "local")
	os.MkdirAll(local, 0755)
	mkfile(t, filepath.Join(remote, "2024/a.jpg"), "hello")
	fakeSSH := writeFakeSSH(t, work, bin, remote)
	cfgPath := writeConfig(t, work, remote, local, fakeSSH)

	before := treeHash(t, local)
	code, _, errb := run("pull", "--config", cfgPath)
	if code != exitcode.PendingChanges {
		t.Fatalf("dry-run pull exit = %d, want 5\n%s", code, errb)
	}
	after := treeHash(t, local)
	// Only .qsync state may change; library content must be identical.
	if before != after {
		t.Fatal("dry-run pull modified the library tree")
	}
}

func TestPullApplyTransfersFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := buildTestBinary(t)
	work := t.TempDir()
	remote := filepath.Join(work, "remote")
	local := filepath.Join(work, "local")
	os.MkdirAll(local, 0755)
	mkfile(t, filepath.Join(remote, "2024/a.jpg"), "hello")
	mkfile(t, filepath.Join(remote, "2024/b.jpg"), "world")
	fakeSSH := writeFakeSSH(t, work, bin, remote)
	cfgPath := writeConfig(t, work, remote, local, fakeSSH)

	code, _, errb := run("pull", "--apply", "--config", cfgPath)
	if code != exitcode.Success {
		t.Fatalf("pull --apply exit = %d, want 0\n%s", code, errb)
	}
	if _, err := os.Stat(filepath.Join(local, "2024/a.jpg")); err != nil {
		t.Errorf("a.jpg not transferred: %v", err)
	}
	// Synced manifest + audit file written.
	if _, err := os.Stat(filepath.Join(local, ".qsync/state/synced.manifest.jsonl")); err != nil {
		t.Errorf("synced manifest missing: %v", err)
	}
	hist, _ := filepath.Glob(filepath.Join(local, ".qsync/history/*-pull.jsonl"))
	if len(hist) == 0 {
		t.Error("no audit file written")
	}
}

// treeHash computes a recursive content hash of a directory tree EXCLUDING the
// .qsync state directory.
func treeHash(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if rel == ".qsync" {
			return fs.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		data, _ := os.ReadFile(p)
		fmt.Fprintf(h, "%s:%x\n", rel, sha256.Sum256(data))
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

func TestRun_ScanDefaultIgnore(t *testing.T) {
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "keep.jpg"), "hello")
	mkfile(t, filepath.Join(root, "sub/.DS_Store"), "dsstore")
	mkfile(t, filepath.Join(root, "sub/._.DS_Store"), "apple_double")

	// Run scan directly via CLI.Run, with config that doesn't exist
	var out, errb bytes.Buffer
	code := Run([]string{"scan", "--root", root}, &out, &errb)
	if code != exitcode.Success {
		t.Fatalf("scan exit = %d\nstdout=%s\nstderr=%s", code, out.String(), errb.String())
	}

	got := out.String()
	if !strings.Contains(got, "keep.jpg") {
		t.Errorf("expected keep.jpg in manifest, got:\n%s", got)
	}
	if strings.Contains(got, ".DS_Store") {
		t.Errorf(".DS_Store was not ignored:\n%s", got)
	}
	if strings.Contains(got, "._.DS_Store") {
		t.Errorf("._.DS_Store was not ignored:\n%s", got)
	}
}

func TestPullApplyWithDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := buildTestBinary(t)
	work := t.TempDir()
	remote := filepath.Join(work, "remote")
	local := filepath.Join(work, "local")
	os.MkdirAll(local, 0755)

	// Remote has only a.jpg
	mkfile(t, filepath.Join(remote, "2024/a.jpg"), "hello")
	// Local has a.jpg and extra.jpg (deleted on remote)
	mkfile(t, filepath.Join(local, "2024/a.jpg"), "hello")
	mkfile(t, filepath.Join(local, "2024/extra.jpg"), "to be deleted")

	fakeSSH := writeFakeSSH(t, work, bin, remote)
	cfgPath := writeConfig(t, work, remote, local, fakeSSH)

	code, _, errb := run("pull", "--apply", "--delete", "--config", cfgPath)
	if code != exitcode.Success {
		t.Fatalf("pull --apply --delete exit = %d, want 0\n%s", code, errb)
	}
	if _, err := os.Stat(filepath.Join(local, "2024/extra.jpg")); !os.IsNotExist(err) {
		t.Errorf("extra.jpg was not deleted by pull --apply --delete")
	}
}


