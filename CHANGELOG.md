# Changelog

All notable changes to qSync are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Added — Phase 1 scope

- Single static Go binary (`CGO_ENABLED=0`), no third-party deps beyond
  `gopkg.in/yaml.v3`.
- Config loading/validation with `~` expansion, `dry_run` defaulting to true,
  unknown-key rejection, and aggregated validation errors.
- Filesystem scanner producing deterministic, sorted JSONL manifests
  (files, directories, symlinks); gitignore-style ignore subset; per-file
  error collection.
- Three-way conflict detection (synced ancestor vs local vs remote), including
  the first-run empty-ancestor case.
- Plan generation with typed change reasons and exact byte totals.
- rsync argument builder (never emits `--delete*`) and `--itemize-changes`
  parser.
- Advisory `flock`-based locking (Linux + macOS) with stale-PID takeover.
- Append-only JSONL audit log per operation.
- Commands: `init`, `doctor` (all environment checks), hidden `scan`,
  `status`, `plan`, `pull`, `push` (with "DGX has newer changes" safety),
  `verify` (`--checksum`), `purge` (typed confirmation + `--yes`).
- Human and `--json` output modes; the JSON envelope keeps stdout clean.
- Exit-code contract (0/1/2/3/4/5).
- Makefile with build/test/vet/release targets; cross-compilation for
  linux/macOS on amd64/arm64.

### Safety

- `TestNoDeleteFlag` guards against any `--delete*` flag ever being constructed.
- Dry-run is the default for every mutating command; `--apply` is authoritative.
