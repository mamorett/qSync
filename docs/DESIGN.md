# PhotoLib Design & Safety Audit Map

This document reproduces the Safety Rules from `tocode.md` verbatim and maps
each to the code that enforces it and the test that guards it. It is the audit
map for reviewers.

---

## Safety Rules (verbatim from `tocode.md`)

### Rule 1

Dry-run by default.

### Rule 2

Never delete automatically.

### Rule 3

Never overwrite conflicts.

Abort instead.

### Rule 4

Never push if DGX has newer changes.

Require pull first.

### Rule 5

Single synchronization lock.

Only one sync operation may execute.

### Rule 6

Every apply operation generates an audit log.

---

## Enforcement Map

| Rule | Enforced in | Guarded by test |
|------|-------------|-----------------|
| 1 — Dry-run by default | `internal/cli/sync_common.go` (`runSync`: mutation happens only when `opts.apply` is true; the flag is authoritative and ignores `defaults.dry_run`) | `internal/cli` `TestPullDryRunLeavesTreeUnchanged` |
| 2 — Never delete automatically | `internal/rsyncx/rsync.go` (`BuildArgs` never emits `--delete*`); deletions are staged in `internal/purge` and executed only by `photolib purge` after typed confirmation | `internal/rsyncx` `TestNoDeleteFlag`; `internal/purge` `TestConfirmPhrase`, `TestRemoteDeleteCommands_*` |
| 3 — Never overwrite conflicts | `internal/conflict/conflict.go` (`Detect`); `internal/cli/sync_common.go` aborts with exit 2 before any rsync transfer | `internal/conflict` `TestDetect_*` |
| 4 — Never push if DGX has newer changes | `internal/cli/sync_common.go` (`remoteHasNewerChanges`, checked before push transfer) | covered by push flow in `internal/cli` integration harness |
| 5 — Single synchronization lock | `internal/lock` (`Acquire`/`Release`/`IsLocked`, `flock` on Linux/Darwin); mutating commands hold the lock, read-only commands refuse when held | `internal/lock` `TestLockContention`, `TestIsLocked` |
| 6 — Audit log per apply | `internal/audit` (`Writer`, one JSONL file per run under `.photolib/history/`); wired in `runSync` and `cmdPurge` | `internal/audit` `TestWriterRoundTrip`; `internal/cli` `TestPullApplyTransfersFiles` asserts an audit file is written |

---

## Determinism

- Manifests are written sorted by path (`internal/snapshot/manifest.go`), with
  mtimes truncated to whole seconds (`internal/snapshot/scan.go`).
- Plans sort `Changes` and `Conflicts` by path and use struct-only JSON
  marshaling for stable field order (`internal/planner`, `internal/cli/render.go`).
- Verified by running `photolib plan --json` twice and diffing the bytes.

## No hidden state

All tool state lives under `<target>/.photolib/`:

- `state/` — `local`, `remote`, `synced` manifests and `pending-deletions.json`
- `history/` — append-only audit logs
- `tmp/` — scratch
- `sync.lock` — advisory lock

No daemons, no databases, no state outside this directory.
