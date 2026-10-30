---
title: "Spec: Agent Fabric core CLI, schema, adapters, and packaging"
type: spec
status: active
covers: cmd/agent-fabric/, internal/agent/, internal/adapter/, internal/manifest/, extensions/pi/, scripts/
last_checked: 2026-10-30
---

# Agent Fabric Specification

## Companion Specifications

- [secret-guard.md](secret-guard.md) — Standalone shell/read guard CLI contract, limits, and private denial audit. Opt-in Pi discovery install is explicit; it is not default enforcement.
- [delegation-and-evaluation.md](delegation-and-evaluation.md) — Fresh-context Delegation Packet contract, pre-implementation evaluation baseline, and external pre-review evidence.
- [supervision-and-delivery.md](supervision-and-delivery.md) — Autonomous execution contract, bounded episodes, delivery ownership and checkout closure, native asynchronous orchestration, phase closure, and bug intake orchestrator.
- [supervision-ledger-and-qa.md](supervision-ledger-and-qa.md) — Deterministic supervision support (`support.py`), discretionary QA and startup contracts, and supervisor ledger hook (`record-ledger.py`), continuity, and curation firewall.
- [task-retrospective.md](task-retrospective.md) — Harness review investigation, verified blockers, scoped correction, and audit contracts.

## Core Schema, Adapters, and Manifest Ownership

Canonical definitions are Markdown files with YAML-like frontmatter. Identity is
the filename stem. `description` and `x-agent-fabric.schema: 1` are required. A
checkout or installed source is recognized only when both `agents/` and
`adapters/` directories are present, so unrelated working-directory folders do
not shadow the bundled source.
Adapters must validate the complete source set before writing anything. Adapter
profiles are loaded from the checked-in mapping and may be partially overridden
by one user file at `~/.config/agent-fabric/config.json` or `AGF_CONFIG`.
Bundled OpenAI profiles use `openai/gpt-6.1-sol` for OpenCode/Kilo readonly,
worker, reviewer, supervisor, planner, qa, recursive, and solver roles, and `gpt-6.1-sol`
for Codex readonly, worker, reviewer, supervisor, and qa roles.
OpenCode/Kilo bundled profiles must not restore retired `openai/gpt-5.6*` defaults
into the dashboard's generated model suggestions.
Profile overrides merge `permissions` and `tools` entries by key. `tools` is a
boolean OpenCode allowlist map; OpenCode 2 compiles it to sorted permission rules,
while Kilo, Claude, Antigravity, and Codex ignore the target-specific field.
Exact-agent OpenCode overrides live under `agents.<target>.<agent>` in the same
user config and may set `model`, `effort`, and merged `tools`. They apply after the
role profile during normal rendering. During
sync, an exact override may update an otherwise unselected Markdown agent only
when its exact target/agent pair is manifest-owned and still resolves to its managed
destination; unknown agent IDs fail before any writes. Project sync ignores an
override owned only by the global manifest, never creating or modifying a
project-local hub file. Exact-agent overrides for non-OpenCode targets are rejected
rather than applied inconsistently.

OpenCode targets the V2 agent contract (verified against 2.0.30): model effort is
encoded as `provider/model#variant` (an explicit variant in model wins), visibility
becomes `hidden`, and permissions are ordered `{action,resource,effect}` rules.
Portable bash/task map to shell/subagent; network covers webfetch/websearch only,
not shell/MCP egress or an OS sandbox. Omit legacy variant/sandbox/visibility/
isolation/permission/tools/hooks frontmatter; resolved hook commands remain in the
body. Tool flags restrict effective actions, with wildcard defaults first and
explicit permission denies retained last. An allow flag cannot override a role deny.
Normalize aliases before ordering; conflicting aliases resolve deny over ask over
allow, independent of input-map iteration. Re-append explicit action denies and
any explicit `external_directory` policy (allow/ask/deny): directory access is a
non-tool guard, not subject to the tool allowlist. Never re-append the wildcard
default or other policy allows, so tool restrictions and deliberate exceptions survive.
Other harness output is unchanged. Exact overrides preserve native permission rules;
tool overrides for an unselected native agent require re-rendering from its owning
source rather than flattening ordered resource scopes into an action map. Model/
effort-only overrides preserve the rules byte-for-byte.
legacy managed hub files retain their old format until refreshed from their owner.

Root `--help`/`-h` lists commands and the subcommand-help entry point. Flag-bearing
subcommands (including `sync` and `hub install`) print flags/defaults to stderr
for `--help`/`-h` and exit zero before loading sources or writing state. Unknown
flags still exit nonzero; help is not a blanket error exemption.

When agent or tool selections are omitted from an interactive command, the CLI
uses `/dev/tty`, preselects all canonical agents and PATH-detected tools, and
accepts comma-separated replacements. Explicit flags or `--yes` are required
for noninteractive execution.

Install state is scoped: global state lives at `~/.config/agent-fabric/` and project
state at `<project>/.agent-fabric/`. The manifest is atomically replaced and records
source, mappings, omissions, generated paths, SHA-256 hashes, and migration actions.
Generated files and the manifest are committed as one guarded operation and rolled
back when a write fails. Modified managed files are never overwritten without
`--force`; files matching their previous manifest hash are eligible for generated
upgrades. `doctor` reports missing files, hash drift, unsafe writable permissions,
invalid managed destinations, and unavailable source mappings.

Antigravity global agents are installed to
`~/.gemini/config/agents/<agent-id>/agent.md`; project-scoped agents use
`<project>/.agents/agents/<agent-id>/agent.md`. The Antigravity adapter renders
the native `name` field. When an adapter destination changes, installation removes
only the previous manifest-owned file whose hash is unchanged, preserving modified
or non-regular prior files as unmanaged artifacts.

### Pi

`--tools pi` renders Markdown for Pi's official optional `subagent` example
extension: global `${PI_CODING_AGENT_DIR:-~/.pi/agent}/agents/<id>.md`, project
`<project>/.pi/agents/<id>.md`. Install/sync/doctor/uninstall use the same manifest
ownership checks as other adapters. The upstream subagent extension is
host-installed, not vendored or silently fetched by Fabric. The optional Fabric
mode extension lives in `extensions/pi` and is installed explicitly as a local Pi
package; see [Pi setup](../runbooks/pi.md). `--assets secret-guard --tools pi`
installs a separate discovery entry at
`${PI_CODING_AGENT_DIR:-~/.pi/agent}/extensions/agf-secret-guard/index.ts`, or
`<project>/.pi/extensions/agf-secret-guard/index.ts` with `--project`, using an
absolute helper path. `--assets secret-guard --tools opencode` installs a
separate discovery entry at
`${OPENCODE_CONFIG_DIR:-${XDG_CONFIG_HOME:-~/.config}/opencode}/plugins/agf-secret-guard/index.ts`,
or `<project>/.opencode/plugins/agf-secret-guard/index.ts` with `--project`.
Ownership, read coverage, and explicit shell non-support:
[secret-guard.md](secret-guard.md).
Only `name`, `description`, `model`, and comma-separated `tools` are emitted.
Bundled profiles use the explicit mapping sentinel `model: inherit`, omitting the
native model field so the child inherits the active parent model and thinking.
An explicit profile model override uses Pi's `provider/model:effort` syntax.
Resolved portable hook commands remain in the body, not fictitious native hook
registrations. Roles denying edit get `read,grep,find,ls`; other roles also get
`bash,edit,write,subagent`. Read-only roles cannot run shell hooks themselves;
the dispatcher must supply required hook evidence or report the capability gap.
Pi tools do not enforce filesystem isolation, resource permissions, or visibility;
rendering records this limitation and the extension dependency as an omission,
so `--strict` refuses installation rather than claiming sandbox equivalence.

The Fabric mode extension uses global rendered definitions as current-session
roles, selected by `/agf-mode` or `--agf-mode`. It appends the selected
body to the host system prompt, preserves session history and model/thinking,
and enforces the definition's exact tool allowlist at tool-call time, including
nested and third-party tools. Unknown host tools are omitted. It never loads
project definitions implicitly. Switching requires idle execution; `off` restores
the pre-mode active tool set, not every registered tool. Selection and baseline
are session entries restored from the active branch on start/tree navigation;
an unavailable restored role fails closed at tool execution until explicitly
replaced or disabled. Mode switching provides neither child-session isolation
nor independent review. Rendered model fields remain subagent-only overrides.

Hub sources must be local directories, local `.tar.gz` archives, or HTTPS. GitHub
repository/tree/tag URLs are converted to archive URLs; HTTPS Git URLs are
shallow-cloned with prompts disabled. Archive downloads have redirect,
compressed-size, extracted-size, per-file-size, regular-file, metadata-header,
and path-traversal limits. PAX/GNU metadata headers are ignored without being
written; links, devices, and other entry types are rejected. `hub.json` must
agree with each definition's dependency metadata. Hub dependencies are offered
interactively and fail before writes in noninteractive mode when Fabric
dependencies are not installed for every selected target.

Canonical agent bodies are the complete portable workflow contract, not role
summaries. They retain role boundaries, decision and evidence requirements,
failure handling, and output schemas. They must not contain provider model IDs,
private Company OS paths, named task-system commands, credentials, or trace/cache
environment variables; adapters and hosts own those integrations.

## Hook Resolution, Codex Adapter, and Release Packaging

Canonical bodies declare registered hooks with `<agent-hooks:list-available>` and
`<agent-hooks:invoke:<event>>` placeholders. During install or sync, the CLI
resolves each registered event once from host-global `~/.agent-hooks/`; Markdown
precedes an executable script. Deterministic hook events include `load-task`, `pre-plan`,
`classify`, `label`, `persist-ticket`, `decompose`, `post-plan`, `record-ledger`, `pre-deploy`, and `post-deploy`.
The generated agent receives either inlined Markdown instructions, an executable script path,
or an explicit no-hook continuation (or section omission for optional lifecycle blocks). Loop-supervisor
delegation hooks use `pre-delegate-<agent>` and `post-delegate-<agent>` events;
absent hooks are omitted so its default generated output is unchanged. The generated
file is therefore deterministic until its next install or sync. Hooks own their
own caching, logging, input transport, and failure semantics; canonical agents
only follow the rendered invocation instruction.

The Codex adapter emits valid role-config values (`workspace-write` for writable
roles) and keeps resolved Agent Fabric hook instructions inside
`developer_instructions`. It does not serialize portable lifecycle names into
Codex's native `hooks` field, which has a different object schema.

Release archives are static cross-platform builds accompanied by
`sha256sums.txt` and a machine-readable `release-manifest.json`. Bootstrap verifies
archive member paths and types before extraction, installs `agent-fabric` and a
copied `agf` alias, removes only its bundled source directories, and never edits
shell profiles. Archives include the optional Pi mode package at `extensions/pi`,
including `secret-guard.ts`, and stage `extensions/opencode/secret-guard.ts`.
Bootstrap stages those files and does not enable the guard or edit Pi or
OpenCode settings. Package installation and `--assets secret-guard` remain
explicit.

`AGF_INSTALL_DIR` is a bootstrap setting for the executable location; the CLI
does not expose a no-op self-install flag.
