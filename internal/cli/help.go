package cli

import "runtime"

func goPlatform() string {
	return runtime.Version() + ", " + runtime.GOOS + "/" + runtime.GOARCH
}

// helpText returns the canonical help text for a command (<30 lines each).
func helpText(c *command) string {
	if t, ok := helpTexts[c.name]; ok {
		return t
	}
	return "Usage: photolib " + c.name + "\n\n" + c.summary + "\n"
}

var helpTexts = map[string]string{
	"init": `photolib init — create a config file and target state dirs

Usage:
  photolib init [--host H] [--source-path P] [--target-path P] [--force]

Flags:
  --host          remote DGX ssh host (user@host)
  --source-path   absolute source path on the DGX
  --target-path   local library root
  --force         overwrite an existing config; create target if missing

Exit codes: 0 ok, 1 error.

Examples:
  photolib init --host dgx --source-path /photos --target-path ~/Pictures
  photolib init --force
`,
	"doctor": `photolib doctor — run environment checks

Usage:
  photolib doctor [--json]

Checks config, rsync/ssh binaries, SSH connectivity, remote paths,
target writability, free disk space, and lock status.

Exit codes: 0 ok (warnings allowed), 1 hard failure.

Examples:
  photolib doctor
  photolib doctor --json | jq .
`,
	"status": `photolib status — show local changes since last sync

Usage:
  photolib status [--json]

Performs a fresh local scan and compares against the last synced manifest.
Does not contact the remote.

Exit codes: 0 nothing pending, 5 local changes pending, 1 error.

Examples:
  photolib status
`,
	"plan": `photolib plan — compute pending changes (no mutation)

Usage:
  photolib plan [--direction pull|push] [--all] [--json]

Scans locally, fetches the remote manifest over SSH, detects conflicts,
and prints the plan. Refreshes state manifests but transfers nothing.

Exit codes: 0 nothing to do, 2 conflicts, 5 changes pending, 1 error.

Examples:
  photolib plan
  photolib plan --direction push --json
`,
	"verify": `photolib verify — verify integrity against the remote

Usage:
  photolib verify [--checksum] [--all] [--json]

Compares size+mtime by default; --checksum also compares SHA-256.

Exit codes: 0 clean, 4 mismatches, 1 error.

Examples:
  photolib verify
  photolib verify --checksum
`,
	"pull": `photolib pull — pull changes from the DGX

Usage:
  photolib pull [--apply] [--wait-lock DUR] [--all] [--json]

Dry-run by default; pass --apply to transfer. Never deletes local files
(rsync runs without --delete). Aborts on conflicts.

Exit codes: 0 ok, 2 conflicts, 3 lock held, 5 pending (dry-run), 1 error.

Examples:
  photolib pull
  photolib pull --apply
`,
	"push": `photolib push — push changes to the DGX

Usage:
  photolib push [--apply] [--wait-lock DUR] [--all] [--json]

Dry-run by default. Aborts if the DGX has newer changes (run pull first).
Deletions are staged to pending-deletions.json for 'photolib purge'.

Exit codes: 0 ok, 2 conflicts, 3 lock held, 5 pending (dry-run), 1 error.

Examples:
  photolib push
  photolib push --apply
`,
	"purge": `photolib purge — execute staged deletions

Usage:
  photolib purge [--yes] [--json]

Reads pending-deletions.json and deletes those files on the DGX after
interactive confirmation (type 'delete N files'). --yes skips the prompt.

Exit codes: 0 ok (or nothing to purge), 1 aborted/error.

Examples:
  photolib purge
  photolib purge --yes
`,
	"scan": `photolib scan — scan a root and emit a manifest (internal)

Usage:
  photolib scan --root PATH [--checksum]

Writes a JSONL manifest to stdout. Used by pull/plan over SSH.
Honors PHOTOLIB_FAKE_ROOT for integration testing.
`,
}
