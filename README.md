# qSync

<p align="center">
  <img src="logo.png" alt="qSync logo" width="240">
</p>

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
2. **Never Delete Automatically**: qSync never passes `--delete` to rsync. Deleted local files are never automatically removed from the remote server. Instead, remote deletions are staged to a pending file and executed *only* when you run `qsync purge` and interactively confirm the deletion.
3. **No Overwriting Conflicts**: If a file has been modified on both the local machine and the remote server since the last sync, qSync aborts the sync and registers a **three-way conflict**. You must resolve it manually before syncing can proceed.
4. **No Push on Outdated State**: If the remote server contains newer updates that you haven't pulled yet, qSync refuses to push, preventing you from overwriting remote changes.
5. **Advisory Synchronization Lock**: A flock-based lock (`sync.lock`) prevents multiple instances of qSync from running concurrently on the same target directory, preventing state corruption.
6. **Immutable Audit Logs**: Every mutating operation (`--apply` or `purge`) appends a detailed JSONL log to `<target>/.qsync/history/` for complete auditability.

---

## 🚀 Quick Installation

### Prerequisites
- **Local Machine**: macOS or Linux with Go (1.22+), `ssh`, and `rsync`.
- **Remote Host**: Linux/Unix server with `ssh` access, `rsync` installed, and the `qsync` executable in the remote user's `PATH` (see [Remote Host Setup](#-remote-host-setup)).

### Install from Go package registry
```bash
go install github.com/yourorg/qsync@latest
```

### Build from Source
```bash
# Clone the repository
git clone https://github.com/yourorg/qsync.git
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
| **[`plan`](#plan)** | Inspect | Compute pending changes (no mutation) | `0` Clean, `2` Conflict, `5` Pending, `1` Error |
| **[`verify`](#verify)** | Inspect | Verify library integrity against remote | `0` Clean, `4` Mismatch, `1` Error |
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
Analyzes change direction.
- **Example (Incoming)**: `qsync plan --direction pull`
- **Example (Outgoing)**: `qsync plan --direction push --json`

### `verify`
Compares file size and mod times.
- **Example (Metadata check)**: `qsync verify`
- **Example (Full SHA-256 validation)**: `qsync verify --checksum`

### `pull`
Synchronizes remote changes down to local.
- **Example (Dry-run)**: `qsync pull`
- **Example (Execution)**: `qsync pull --apply --wait-lock 2m`

### `push`
Synchronizes local changes up to remote.
- **Example (Dry-run)**: `qsync push`
- **Example (Execution)**: `qsync push --apply`

### `purge`
Triggers permanent deletions of remote files that were deleted locally.
- **Example (Interactive)**: `qsync purge`
- **Example (Scripted/Auto)**: `qsync purge --yes`

---

## ⚙️ Configuration Guide (`config.yaml`)

By default, configuration is stored in `~/.config/qsync/config.yaml`. You can specify a custom configuration path using the global `--config` flag.

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
  dry_run: true           # If true, push/pull require --apply to modify files

# Optimization & tuning for rsync operations
rsync:
  extra_args:
    - --compress
    - --copy-links
  bwlimit_kb: 5000        # Limit bandwidth to 5000 KB/s (0 = unlimited)

# List of ignore glob patterns
ignore:
  - Thumbs.db
  - "*.tmp"
  - ".DS_Store"
  - "cache/"
```

---

## 🚫 Ignore Patterns Syntax

The `ignore:` list in `config.yaml` is highly optimized. It uses a subset of Gitignore syntax evaluated per path segment:

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
1. It exits with code `2` and displays a detailed list of conflict files.
2. It refuses to transfer any files to prevent overriding either version.

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

### Example Sync Cron Script
```bash
#!/usr/bin/env bash
set -euo pipefail

TARGET_DIR="$HOME/Pictures"

echo "Checking environment..."
qsync doctor

# Check if there are changes to pull
if qsync plan --direction pull > /dev/null; then
  echo "No remote changes to pull."
else
  # Exit code 5 indicates pending changes
  echo "Pulling remote changes..."
  qsync pull --apply
fi

# Check if there are changes to push
if qsync plan --direction push > /dev/null; then
  echo "No local changes to push."
else
  echo "Pushing local changes..."
  qsync push --apply
fi

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
│   └── history/
│       ├── 20260720-120000-pull.jsonl  # Append-only pull audit log
│       └── 20260720-123000-push.jsonl  # Append-only push audit log
└── your_photos/
    ├── photo1.jpg
    └── photo2.jpg
```