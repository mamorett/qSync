# PhotoLib

<p align="center">
  <img src="logo.png" alt="PhotoLib logo" width="240">
</p>

A safe, deterministic synchronization tool for photo libraries built on rsync over SSH.

## Quick Installation

```bash
# Install from GitHub releases
go install github.com/yourorg/photolib@latest

# Or build from source
cd photolib
make build
```

## Quick Start

```bash
# Initialize configuration
photolib init --host dgx --source-path /photos --target-path ~/Pictures

# Check environment
photolib doctor

# Inspect changes
photolib plan
photolib status

# Sync changes
photolib pull --apply

# Work locally...

# Push changes
photolib push --apply

# Purge deleted files
photolib purge
```

## Workflow

The only supported workflow is:

1. **Pull**: `git pull`-like operation to get latest from source of Truth
2. **Work locally**: Edit, organize photos
4. **Push**: Send updates back to Source of Truth
5. **Purge**: Remove deleted files from Source of Truth

Deletions are **never automatic** - files deleted from your local library are NOT deleted from the source. You must run `photolib purge` to remove them from the DGX.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0    | Success |
| 1    | Generic Error |
| 2    | Conflicts detected |
| 3    | Lock is held |
| 4    | Verification failed |
| 5    | Pending changes detected |

## JSON Mode

All commands support `--json` for machine-readable output:

```bash
photolib plan --json | jq
```

In JSON mode:
- stdout contains only the JSON document
- stderr contains human diagnostics
- Scripts can reliably parse command output

## Configuration

Configuration file: `~/.config/photolib/config.yaml` or specified via `--config`

```yaml
target:
  path: ~/Pictures
source:
  host: dgx
  path: /photos
transport:
  port: 22
defaults:
  dry_run: true
rsync:
  extra_args:
    - --compress
```

See `photolib init --help` for flag-based setup.

## Deletions are never automatic

**PhotoLib never passes `--delete` to rsync in any code path.** A file removed
from your local library is *not* removed from the DGX by `push`. Instead the
deletion is recorded in `<target>/.photolib/state/pending-deletions.json` and
executed only when you run `photolib purge` and confirm interactively (type
`delete N files`, or pass `--yes` in scripts). The same holds in reverse: `pull`
never removes local files that are absent on the DGX.

This is a deliberate safety control against accidental mass deletion. See
`docs/DESIGN.md` for the code locations that enforce it (and the
`TestNoDeleteFlag` safety test).

## Remote Setup

The DGX (source of truth) must have the `photolib` binary installed on its
`PATH`. PhotoLib produces the remote manifest by running
`ssh <host> photolib scan --root <source.path>` — it does not shell out to a
bespoke remote protocol.

```bash
# On the DGX, install the same version you run locally:
GOOS=linux GOARCH=amd64 go build -o photolib ./cmd/photolib
scp photolib dgx:/usr/local/bin/photolib
# verify:
photolib doctor        # the "remote-photolib" check must pass
```

If the remote binary is missing, `pull`/`plan`/`push`/`verify` fail with a
clear message. (An rsync-`--itemize-changes` fallback is a future stretch goal
and is not implemented.)

## Ignore Patterns

`ignore:` in the config uses a documented *subset* of gitignore syntax:

- literal names (`Thumbs.db`) — match that name in any directory
- `*`/`?`/`[…]` globs, evaluated per path segment via `path.Match` (`*.tmp`)
- a trailing `/` (`cache/`) matches directories only
- a leading `/` (`/2024/raw`) anchors the pattern to the library root

The `.photolib` state directory is always excluded automatically.

## Determinism

Identical inputs produce byte-identical plans and rsync invocations. Manifests
are written sorted by path with mtimes truncated to whole seconds. You can
verify: `photolib plan --json` run twice yields identical bytes.

## FAQ

- **Why not `rsync --delete`?** A single mistaken local deletion would propagate
  destructively. Deletions are staged and require explicit confirmation.
- **Why not bidirectional auto-sync?** Automatic merge logic hides conflicts.
  PhotoLib detects three-way conflicts and refuses to guess; you resolve them.
- **How do I script it?** Rely on the exit codes. Example:
  `photolib plan >/dev/null; [ $? -eq 5 ] && photolib pull --apply`.

## Safety Rules

1. Single Go binary - CGO disabled
2. Dry-run by default - explicit `--apply` required for mutations
3. Deletions are never automatic - use `purge` command
4. Locking mechanism prevents overlapping syncs
5. Conflict detection prevents data corruption