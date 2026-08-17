# Agent Fabric

Agent Fabric installs eight portable agents into OpenCode, Kilo, Antigravity CLI,
Codex, and Claude Code. It copies generated files and records ownership in a
manifest so upgrades do not overwrite user edits.

## Requirements

Linux or macOS on amd64 or arm64. The bootstrap needs `curl`, `tar`, and either
`sha256sum` or `shasum`. Windows archives are produced by the release workflow.

## Install

Run this on Linux or macOS:

```sh
curl -fsSL https://ericmaster.ninja/agent-fabric/install | bash
```

For a headless install, pass all selections explicitly:

```sh
curl -fsSL https://ericmaster.ninja/agent-fabric/install | \
  bash -s -- --all --tools opencode,kilo,agy,codex,claude --yes
```

The bootstrap verifies the release checksum, installs `agent-fabric` and the `agf`
alias into `~/.local/bin`, then starts the setup prompt. It does not edit shell
profiles, authenticate with a provider, or make a model request.

If `agf` is not found, use it immediately with:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Add that directory to your shell configuration yourself if you want it to persist.

The CLI uses `/dev/tty` when selections are omitted. The interactive prompt
preselects all built-in agents and tools found on `PATH`; enter comma-separated
IDs to change either selection, including tools not currently installed.

## Verify

Run these commands after installation:

```sh
agf --version
agf validate
agf list
agf doctor
```

`validate` checks the canonical source and all adapter mappings. `doctor` checks
the manifest, generated-file hashes, file permissions, and mapping availability.
The installed source is bundled beside the executable, so these commands work
outside a checkout.

## Install And Sync

Global installation is the default. Use `--project` for files inside a specific
repository. Generated paths are the native agent directories for each target.

```sh
agf install --all --tools opencode,kilo --yes
agf install --project /path/to/repo --agents planner,qa-runner --tools kilo --yes
agf sync --all --tools opencode,kilo --yes
```

The global manifest is `~/.config/agent-fabric/.agent-fabric-manifest.json`. A
project manifest is `<project>/.agent-fabric/.agent-fabric-manifest.json`.
`sync` is idempotent. Files are copied, never symlinked.

If a managed file still matches its manifest hash, `sync` updates it. A file
edited by the user is preserved and the command reports its path. Use `--force`
only when replacing that edit is intentional:

```sh
agf sync --all --tools opencode --yes --force
agf uninstall
agf uninstall --force
```

Without `--force`, uninstall removes only unchanged manifest-owned files and
keeps modified files. If a command fails, inspect `agf doctor`, review the
reported paths, and rerun with explicit selections. No shell profile changes
are made automatically.

## Hub Agents

Hub installation is explicit and never part of the public bootstrap. Install the
public catalog after Fabric is available:

```sh
agf hub install https://github.com/ericmaster/agent-hub \
  --tools opencode,kilo --yes
```

`simplification-planner` requires the Fabric `planner` agent. Interactive hub
installation offers to install a missing dependency; noninteractive installation
fails before writing and tells you to install Fabric first. Name collisions and
invalid dependency graphs are rejected before any generated file is written.
Local paths, HTTPS tarballs, GitHub repository/tree/tag URLs, and HTTPS Git URLs
are accepted as hub sources.

## Overrides And Migration

The bootstrap accepts these environment variables:

```sh
AGF_VERSION=1.0.0
AGF_REPOSITORY=owner/agent-fabric
AGF_INSTALL_DIR="$HOME/bin"
```

`AGF_INSTALL_DIR` controls where the bootstrap places the executable. The CLI
does not move or reinstall its own executable.

Optional target profile overrides live in one user file,
`~/.config/agent-fabric/config.json` (or `AGF_CONFIG`):

```json
{
  "profiles": {
    "opencode": {
      "worker": {
        "model": "openai/gpt-5.6",
        "effort": "high",
        "permissions": { "network": "deny" }
      }
    }
  }
}
```

Use `--source /path/to/agent-fabric` when testing a local checkout. Migration
removes only known legacy symlink entries by default; known regular legacy entries
require `--force`. Unrelated files and unknown entries are preserved.

## Canonical Contract

Each `agents/<id>.md` derives identity from its filename and contains a required
description, OpenCode-compatible mode, and `x-agent-fabric` schema version 1
block. The block carries logical `profile`, `effort`, tool-neutral permissions,
visibility, isolation, hooks, and optional dependencies. Provider model IDs live
only in `adapters/<target>.json`.

The six portable hook events are `load-task`, `pre-plan`, `classify`, `label`,
`decompose`, and `post-plan`. Hooks are documentation contracts; implementations,
task systems, credentials, and execution runtimes remain host-owned. Registered
events are resolved once at session start and cached at
`${AGENT_TRACE_ROOT}/hook-capabilities.json`. A missing capability is reported
once as `CAPABILITY_UNAVAILABLE`, and local artifacts are preserved.

## Release And Maintainer Checks

```sh
scripts/release.sh 1.0.0 dist
./scripts/release-gate.sh
```

The release script builds static Linux amd64/arm64, macOS amd64/arm64, and
Windows amd64 archives, `sha256sums.txt`, and `release-manifest.json`. The release
gate fails closed unless an explicitly approved disposable Proxmox target is
configured, and it destroys only the claimed validation container after proving
cleanup.
