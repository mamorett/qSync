# qSync

<p align="center">
  <img src="logo.png" alt="qSync logo" width="240">
</p>

[![Go Reference](https://pkg.go.dev/badge/github.com/mamorett/qsync.svg)](https://pkg.go.dev/github.com/mamorett/qsync)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![GitHub](https://img.shields.io/badge/GitHub-mamorett/qsync-181717.svg)](https://github.com/mamorett/qsync)

A safe, deterministic unidirectional synchronization tool for file libraries (such as photos, documents, and media archives) built on `rsync` over SSH.

---

## 📖 Table of Contents
1. [Core Philosophy & Safety Guarantees](#-core-philosophy--safety-guarantees)
2. [Quick Installation](#-quick-installation)
3. [Quick Start & Standard Workflow](#-quick-start--standard-workflow)
4. [Detailed Command Reference](#-detailed-command-reference)
5. [Configuration Guide (`config.yaml`)](#-configuration-guide-configyaml)
6. [Ignore Patterns Syntax](#-ignore-patterns-syntax)
7. [Conflict Detection & Resolution Guide](#-conflict-detection--resolution-guide)
8. [Automation & Scripting Guide](#-automation--scripting-guide)
9. [Remote Host Setup](#-remote-host-setup)
10. [Under the Hood: State Directory (`.qsync`)](#-under-the-hood-state-directory-qsync)

---

## 🛡️ Core Philosophy & Safety Guarantees

qSync is designed specifically for **valuable personal archives** where data loss is not an option. It enforces the following strict safety principles:

1. **Dry-Run by Default**: Mutation operations (`pull`, `push`) do not modify anything unless you explicitly pass the `--apply` flag.
2. **No Automatic Deletions**: By default, qSync never passes `--delete` to rsync. Deleted local files are never automatically removed from the remote server. Instead, remote deletions are staged to a pending file and executed *only* when you run `qsync purge` and confirm the deletion. Passing the explicit `--delete` flag to `pull`/`push` is an opt-in that makes rsync remove files on the destination that no longer exist on the source.
3. **No Overwriting Conflicts**: If a file has been modified on both the local machine and the remote server since the last sync, qSync aborts the sync and registers a **three-way conflict**. You must resolve it manually before syncing can proceed.
4. **No Push on Outdated State**: If the remote server contains newer updates that you haven't pulled yet, qSync refuses to push, preventing you from overwriting remote changes.
5. **Advisory Synchronization Lock**: `pull` and `push` acquire an exclusive `flock` on `.qsync/sync.lock` for the duration of their run (with stale-PID takeover), so two syncs can't mutate the same target concurrently. `plan` and `verify` refuse to run while the lock is held (exit `3`), and `status` reports whether the lock is held.
6. **Immutable Audit Logs**: Every `pull`/`push` run (dry-runs included, recorded with `dry_run: true`) and every `purge` appends a detailed JSONL log to `<target>/.qsync/history/` for complete auditability.

---

## 🚀 Quick Installation

### Prerequisites
- **Local Machine**: macOS or Linux with Go (1.26+), `ssh`, and `rsync`.
- **Remote Host**: Linux/Unix server with `ssh` access, `rsync` installed, and the `qsync` executable in the remote user's `PATH` (see [Remote Host Setup](#-remote-host-setup)).

### Install from Go package registry
```bash
go install github.com/mamorett/qsync@latest
```

### Build from Source
```bash
# Clone the repository
git clone https://github.com/mamorett/qsync.git
cd qsync

# Build the executable
make build

# Install the binary to $GOPATH/bin
make install
```

---

## 🔄 Quick Start & Standard Workflow

The typical lifecycle of a synchronization session involves these steps:

### 1. Initialize Configuration
Set up your connection parameters. This creates a configuration file in `~/.config/qsync/config.yaml`.
```bash
qsync init --host user@remote-server --source-path /data/photos --target-path ~/Pictures
```

### 2. Verify Your Environment
Ensure that the local and remote environments are healthy (binaries are found, SSH works, permissions are correct).
```bash
qsync doctor
```

### 3. Check for Pending Incoming Changes (Pull Dry-Run)
Inspect what files will be copied or updated from the remote host.
```bash
qsync pull
```
*Tip: If there are pending changes, `qsync pull` exits with code `5`. If it's clean, it exits with `0`.*

### 4. Fetch and Apply Incoming Changes
Download the new files to your local target folder.
```bash
qsync pull --apply
```

### 5. Work Locally
Now, you can safely work on your files locally: edit photos, organize folders, rename or delete files.

### 6. Inspect Outgoing Changes (Push Dry-Run)
See what changes you are about to send back to the remote server.
```bash
qsync push
```

### 7. Upload Changes to Remote
Upload new/updated files to the remote server.
```bash
qsync push --apply
```
*Note: Any local deletions you performed will NOT be deleted on the remote yet. They are staged.*

### 8. Finalize Deletions (Purge)
Apply the staged deletions to the remote host after a safety prompt.
```bash
qsync purge
```

---

## 💻 Detailed Command Reference

| Command | Group | Summary | Default Exit Codes |
| :--- | :--- | :--- | :--- |
| **[`init`](#init)** | Setup | Create a configuration file and local state directories | `0` OK, `1` Error |
| **[`doctor`](#doctor)** | Setup | Run environment diagnostics and connectivity checks | `0` OK, `1` Hard Failure |
| **[`status`](#status)** | Inspect | Show local changes since last sync (fully offline) | `0` Clean, `5` Pending, `1` Error |
| **[`plan`](#plan)** | Inspect | Compute pending changes (no mutation) | `0` Clean, `2` Conflict, `3` Locked, `5` Pending, `1` Error |
| **[`verify`](#verify)** | Inspect | Verify library integrity against remote | `0` Clean, `3` Locked, `4` Mismatch, `1` Error |
| **[`pull`](#pull)** | Sync | Pull changes from remote (dry-run unless `--apply`) | `0` OK, `2` Conflict, `3` Locked, `5` Dry-run, `1` Error |
| **[`push`](#push)** | Sync | Push changes to remote (dry-run unless `--apply`) | `0` OK, `2` Conflict, `3` Locked, `5` Dry-run, `1` Error |
| **[`purge`](#purge)** | Sync | Execute staged deletions on remote (requires confirmation) | `0` OK, `1` Aborted/Error |

### `init`
Creates the configuration file and structures the `.qsync/` state directory under target path.
- **Example**: `qsync init --host backupsrv --source-path /var/lib/media --target-path ~/Media`

### `doctor`
Performs connection tests and checks dependencies.
- **Example**: `qsync doctor`
- **JSON Output**: `qsync doctor --json`

### `status`
Compares the current files in target path with `.qsync/state/synced.manifest.jsonl`.
- **Example**: `qsync status`

### `plan`
Analyzes change direction and checks for 3-way conflicts.
- **Example (Incoming)**: `qsync plan --direction pull`
- **Example (VFAT/VeraCrypt target)**: `qsync plan --direction pull --vfat`
- **Example (Outgoing)**: `qsync plan --direction push --json`

### `verify`
Compares file size and mod times against the remote.
- **Example (Metadata check)**: `qsync verify`
- **Example (Full SHA-256 validation)**: `qsync verify --checksum`
- **Example (List all mismatches)**: `qsync verify --all`
- **When to use `--checksum`**: Performs a full SHA-256 hash comparison in addition to the size/mtime check. Useful for verifying integrity after a potentially interrupted transfer. Note that `verify` always compares mtime exactly (no VFAT tolerance is applied here); use `--vfat` on `pull`/`push`/`plan` instead when timestamps on FAT-formatted volumes are unreliable.

### `pull`
Synchronizes remote changes down to local.
- **Example (Dry-run)**: `qsync pull`
- **Example (VFAT / VeraCrypt target)**: `qsync pull --vfat --apply`
- **Example (Execution)**: `qsync pull --apply --wait-lock 2m`
- **`--delete`**: Also delete local files that no longer exist on the remote (opt-in; off by default).

### `push`
Synchronizes local changes up to remote.
- **Example (Dry-run)**: `qsync push`
- **Example (VFAT / VeraCrypt target)**: `qsync push --vfat --apply`
- **Example (Execution)**: `qsync push --apply`
- **`--delete`**: Immediately delete remote files that no longer exist locally, instead of staging them for `qsync purge` (opt-in; off by default).

### `purge`
Triggers permanent deletions of remote files that were deleted locally.
- **Example (Interactive)**: `qsync purge`
- **Example (Scripted/Auto)**: `qsync purge --yes`

---

## ⚙️ Configuration Guide (`config.yaml`)

By default, configuration is stored in the user config directory: `~/.config/qsync/config.yaml` on Linux and `~/Library/Application Support/qsync/config.yaml` on macOS. You can override the location with the global `--config` flag or the `QSYNC_CONFIG` environment variable.

### Full Configuration Example
```yaml
# Remote server information
source:
  host: user@remote-dgx
  path: /absolute/path/on/remote/server

# Local library path (supports tilde expansion)
target:
  path: ~/Pictures

# Executable locations and connection options
transport:
  ssh: /usr/bin/ssh       # Optional: Custom path to ssh binary
  rsync: /usr/bin/rsync   # Optional: Custom path to rsync binary
  port: 22                # Optional: Custom SSH port

# Default client behavior flags
defaults:
  dry_run: true           # Advisory. Mutating commands (pull/push) always require --apply to modify files
  vfat: true              # Enable VFAT/exFAT/VeraCrypt mode (2s mtime window, 1h/2h timezone tolerance, suppress perms)
  modify_window: 2        # Custom mtime window in seconds (optional)

# Optimization & tuning for rsync operations
rsync:
  extra_args:
    - --compress
    - --copy-links
  bwlimit_kb: 5000        # Limit bandwidth to 5000 KB/s (0 = unlimited)

# Custom ignore glob patterns (merged with system defaults like .DS_Store)
ignore:
  - Thumbs.db
  - "*.tmp"
  - ".DS_Store"
  - "cache/"
```

---

## 🚫 Ignore Patterns Syntax

System noise files are **automatically ignored by default** across all operations:
- `.DS_Store`
- `._*` (macOS AppleDouble metadata files)
- `Thumbs.db`
- `*.tmp`
- `@eaDir` (Synology metadata directories)

You can add additional custom patterns under the `ignore:` list in `config.yaml`. The ignore syntax uses a subset of Gitignore syntax evaluated per path segment:

- **Literal Match**: `Thumbs.db` matches that exact file name anywhere in the library.
- **Wildcard Glob**: `*.tmp` matches any file ending with `.tmp` in any folder.
- **Directory Exclusion**: A trailing slash (e.g., `cache/`) skips the folder named `cache` and all of its contents.
- **Anchored Pattern**: A leading slash (e.g., `/temp/raw`) anchors the path relative to the library root.

> [!NOTE]
> The internal `.qsync` state directory is **always** excluded from synchronization. You do not need to add it to your ignore list.

---

## ⚠️ Conflict Detection & Resolution Guide

### What is a conflict?
A conflict occurs when a file has been modified on **both** the local machine and the remote server since the last successful sync. 

### How qSync Handles Conflicts
When qSync detects a conflict during `plan`, `pull`, or `push`:
1. It exits with code `2` and displays a detailed list of conflict files with exact diagnostic details (e.g., `CONFLICT: both sides modified (mtime diff 3600s: local=19:04:05, remote=18:04:05)` or `size mismatch`).
2. It refuses to transfer any files to prevent overwriting either version.

### Syncing to FAT32 / exFAT / VeraCrypt Volumes (`--vfat`)
When syncing to FAT32, exFAT, or VeraCrypt containers mounted on macOS, filesystems round timestamps to 2 seconds and store timestamps in Local Time (causing 1-hour or 2-hour UTC offsets). 

Pass the `--vfat` flag (or set `vfat: true` in `config.yaml`):
```bash
qsync pull --vfat --apply
```
This enables a 2-second timestamp tolerance, handles 1h/2h FAT timezone shifts, suppresses permission warnings (`--no-owner --no-group`), and prevents false conflict reports.

### How to Resolve Conflicts Manually
Because qSync is unidirectional, you must decide which side should win and align them before syncing again.

#### Scenario A: Remote changes are correct (Discard local changes)
If you want to keep the remote version and discard your local modifications:
1. Delete or rename the conflicting file locally.
   ```bash
   mv ~/Pictures/conflicted.jpg ~/Pictures/conflicted.jpg.local_backup
   ```
2. Pull the remote version:
   ```bash
   qsync pull --apply
   ```

#### Scenario B: Local changes are correct (Force local changes to Remote)
If you want your local version to override the remote:
1. Make a copy of your local file.
2. Run `qsync pull --apply` (since you moved/deleted the conflict, it will pull the remote version).
3. Copy your local version back over the pulled version.
4. Run `qsync push --apply` to update the remote.

---

## 🤖 Automation & Scripting Guide

qSync is designed to integrate seamlessly into shell scripts, cron jobs, and backup pipelines.

### Exit Code Specifications
Utilize these return codes in your scripting loops:
- `0`: Success (or dry-run with no changes)
- `1`: Fatal usage or configuration error
- `2`: Divergent conflicts found
- `3`: Locking failure (another sync is active)
- `4`: Verification mismatch (`verify` command failed)
- `5`: Pending changes detected (dry-run mode)
- `130`: Interrupted by a signal (`SIGINT`/`SIGTERM`); any partial transfer is reported but state stays consistent

### Example Sync Cron Script
```bash
#!/usr/bin/env bash
set -euo pipefail

TARGET_DIR="$HOME/Pictures"

echo "Checking environment..."
qsync doctor

# Check if there are changes to pull
case "$(qsync plan --direction pull; echo $?)" in
  0) echo "No remote changes to pull." ;;
  5) echo "Pulling remote changes..." ; qsync pull --apply ;;
  2) echo "Conflicts detected - please resolve manually before syncing." ;;
  *) echo "Error during plan check (exit code $?)." ;;
esac

# Check if there are changes to push
case "$(qsync plan --direction push; echo $?)" in
  0) echo "No local changes to push." ;;
  5) echo "Pushing local changes..." ; qsync push --apply ;;
  2) echo "Conflicts detected - please resolve manually before syncing." ;;
  *) echo "Error during plan check (exit code $?)." ;;
esac

# Finalize any staged deletions non-interactively
qsync purge --yes
```

---

## 🖥️ Remote Host Setup

The remote host (the source of truth) must have the `qsync` binary installed and accessible in its SSH path. qSync runs the remote scanner via SSH commands like `ssh <host> qsync scan --root <source.path>`.

### Step-by-Step Installation on Remote Host
1. Compile the binary for the remote operating system/architecture (e.g., Linux amd64):
   ```bash
   GOOS=linux GOARCH=amd64 go build -trimpath -o qsync ./cmd/qsync
   ```
2. Copy the binary to the remote server:
   ```bash
   scp qsync user@remote-host:/usr/local/bin/qsync
   ```
3. Verify that the binary can be found and executed in a non-interactive shell:
   ```bash
   ssh user@remote-host qsync version
   ```
4. Run `qsync doctor` locally. The check `remote-qsync` must pass.

---

## 🗂️ Under the Hood: State Directory (`.qsync`)

All state is stored within the local library target path inside the hidden `.qsync/` folder. This guarantees that qSync runs without external databases or background daemons.

```
<target>/
├── .qsync/
│   ├── sync.lock                   # flock advisory sync lock
│   ├── state/
│   │   ├── local.manifest.jsonl    # Manifest of last local scan
│   │   ├── remote.manifest.jsonl   # Manifest of last remote scan
│   │   ├── synced.manifest.jsonl   # Manifest of last successful sync state
│   │   └── pending-deletions.json  # Staged file deletions waiting for purge
│   ├── history/
│   │   ├── 20260720-120000-pull.jsonl  # Append-only pull audit log
│   │   └── 20260720-123000-push.jsonl  # Append-only push audit log
│   └── tmp/                        # Scratch space (transfer lists, temp files)
└── your_photos/
    ├── photo1.jpg
    └── photo2.jpg
```

---

## 🤝 Contributing

Contributions are welcome! Please feel free to submit pull requests, open issues, or suggest features. See the [LICENSE](LICENSE) for licensing details.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
```