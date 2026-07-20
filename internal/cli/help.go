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
	return "Usage: qsync " + c.name + "\n\n" + c.summary + "\n"
}

var helpTexts = map[string]string{
	"init": `qsync init — create a configuration file and target state directories

Usage:
  qsync init [--host H] [--source-path P] [--target-path P] [--force]

Description:
  Initializes a new target repository for synchronization. It creates a local
  configuration file at ~/.config/qsync/config.yaml (unless --config is specified)
  and prepares the target state directory (.qsync) inside the local target-path.
  If directories do not exist, they will be created automatically.

Flags:
  --host          Remote SSH host (e.g. user@remote-dgx)
  --source-path   Absolute path of the library on the remote host
  --target-path   Absolute path to your local library root folder
  --force         Overwrite an existing config file and recreate state directories

Exit codes:
  0  Success
  1  Usage error or creation failure

Examples:
  qsync init --host user@dgx --source-path /photos --target-path ~/Pictures
  qsync init --force
`,
	"doctor": `qsync doctor — run environment checks and verify remote setup

Usage:
  qsync doctor [--json]

Description:
  Checks if your local and remote environments are correctly configured.
  It verifies:
    - Valid local configuration and correct paths.
    - Presence and execution permissions of local rsync and ssh binaries.
    - Remote SSH connectivity and credentials.
    - Remote qsync executable existence in the remote user's PATH.
    - Write permissions on the target directory.
    - Available disk space on local storage.
    - Active advisory locks on the target.

Flags:
  --json   Output result as machine-readable JSON structure

Exit codes:
  0  All checks pass (warnings allowed)
  1  One or more hard errors occurred (e.g., connection lost, missing binaries)

Examples:
  qsync doctor
  qsync doctor --json | jq .
`,
	"status": `qsync status — show local modifications since the last sync operation

Usage:
  qsync status [--json]

Description:
  Performs a fast local file system scan and compares it with the local sync
  manifest (synced.manifest.jsonl) stored during the last pull/push.
  This identifies files added, updated, or deleted locally. It is entirely
  offline and does not communicate with the remote server.

Flags:
  --json   Render changes list in JSON format

Exit codes:
  0  Clean; no local changes pending sync
  5  Local changes exist (needs planning and push)
  1  Execution error

Examples:
  qsync status
  qsync status --json
`,
	"plan": `qsync plan — compute pending sync changes (read-only, no mutations)

Usage:
  qsync plan [--direction pull|push] [--all] [--json]

Description:
  Performs a three-way diff between the local state, remote state, and the
  last synced manifest to construct a mutation plan.
  It identifies:
    - Added, updated, and deleted files on both sides.
    - Divergent changes (conflicts where both sides modified the same file).
  No files are transferred, and no deletions are performed.

Flags:
  --direction   Direction to analyze: 'pull' (default) or 'push'
  --all         Force full scan (disables caching optimizations)
  --json        Output the complete calculated plan in JSON

Exit codes:
  0  Nothing to do
  2  Conflicts detected (must resolve before sync)
  5  Pending sync changes exist
  1  Error encountered

Examples:
  qsync plan --direction pull
  qsync plan --direction push --json
`,
	"verify": `qsync verify — verify integrity between local and remote directories

Usage:
  qsync verify [--checksum] [--all] [--json]

Description:
  Performs a direct comparison between local and remote directory contents.
  By default, it compares file sizes and modification times.
  With --checksum, it calculates and compares SHA-256 hashes of all files.

Flags:
  --checksum  Calculate and verify SHA-256 content hashes (slow but thorough)
  --all       Do not optimize; compare all files
  --json      Render verify report as JSON

Exit codes:
  0  Success; contents are identical
  4  Verification failed (file size, mtime, or checksum mismatch)
  1  Error encountered

Examples:
  qsync verify
  qsync verify --checksum
`,
	"pull": `qsync pull — download remote library changes locally

Usage:
  qsync pull [--apply] [--wait-lock DUR] [--all] [--json]

Description:
  Downloads new and modified files from the remote source library.
  By default, it runs in dry-run mode. Pass --apply to perform actual transfers.
  Safety:
    - Never deletes local files. Local files deleted remote are kept.
    - Aborts immediately if conflicts exist.

Flags:
  --apply         Execute files transfer (default is dry-run)
  --wait-lock D   Wait up to duration D (e.g. 5m) for active lock to release
  --all           Force full scanning (no optimization)
  --json          Output JSON details of the dry-run/apply operation

Exit codes:
  0  Success (when --apply is used and succeeds)
  2  Aborted due to conflicts
  3  Lock active on target
  5  Pending changes (when run without --apply)
  1  Error encountered

Examples:
  qsync pull
  qsync pull --apply
`,
	"push": `qsync push — upload local library changes to the remote source

Usage:
  qsync push [--apply] [--wait-lock DUR] [--all] [--json]

Description:
  Uploads new and modified files from the local target to the remote host.
  By default, it runs in dry-run mode. Pass --apply to execute transfers.
  Safety:
    - Aborts if the remote has newer updates (run pull first).
    - Deletions are staged to pending-deletions.json and not deleted
      immediately on remote. Use 'qsync purge' to apply deletions.

Flags:
  --apply         Execute files transfer (default is dry-run)
  --wait-lock D   Wait up to duration D (e.g. 10s) for active lock
  --all           Force full scanning
  --json          Output JSON details of the push operation

Exit codes:
  0  Success (when --apply is used and succeeds)
  2  Aborted due to conflicts
  3  Lock active on target
  5  Pending changes (when run without --apply)
  1  Error encountered

Examples:
  qsync push
  qsync push --apply
`,
	"purge": `qsync purge — execute staged remote deletions

Usage:
  qsync purge [--yes] [--json]

Description:
  Reads pending-deletions.json file and permanently deletes those files
  on the remote server. For safety, this requires interactive confirmation
  unless --yes is provided.

Flags:
  --yes   Skip interactive prompt (useful for scripting)
  --json  Output deletion operations result as JSON

Exit codes:
  0  Success (or nothing to purge)
  1  Aborted by user or error occurred

Examples:
  qsync purge
  qsync purge --yes
`,
	"scan": `qsync scan — scan a root and emit a manifest (internal)

Usage:
  qsync scan --root PATH [--checksum]

Description:
  Scans all files in a root directory recursively and writes a sorted JSONL
  manifest to stdout. This is an internal command used remotely via SSH.
  Honors QSYNC_FAKE_ROOT for integration testing.
`,
}
