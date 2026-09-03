# Agent Fabric Specification

Canonical definitions are Markdown files with YAML-like frontmatter. Identity is
the filename stem. `description` and `x-agent-fabric.schema: 1` are required. A
checkout or installed source is recognized only when both `agents/` and
`adapters/` directories are present, so unrelated working-directory folders do
not shadow the bundled source.
Adapters must validate the complete source set before writing anything. Adapter
profiles are loaded from the checked-in mapping and may be partially overridden
by one user file at `~/.config/agent-fabric/config.json` or `AGF_CONFIG`.

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

## Fresh-Context Delegation Contract

Direct user invocation of a dispatcher-capable primary or all-mode role is not a fresh-child handoff and does not require an intake Delegation Packet.
It may perform its ordinary workflow and repository inspection in the
user-selected execution context so it can construct outgoing child packets. If that role is invoked as a fresh child, it validates the intake packet before substantive work.
Every outgoing fresh-child dispatch requires a separately constructed and validated Delegation Packet.

Every explicit fresh-child handoff is self-locating and fail-closed. Before
substantive child work, the producer supplies a Delegation Packet containing:

- the declared execution root plus workspace ownership/isolation and VCS revision
  and working-tree state;
- the objective, explicit non-goals, and bounded scope;
- every authoritative input inline or by authoritative locator;
- permitted source and evidence paths;
- exact commands, observable DoD, and required evidence;
- the rollback boundary; and
- explicit behavior for an unresolved locator.

A **Declared Root** is a packet-named execution, artifact, or evidence base. A
relative locator explicitly names the Declared Root or base against which it is
resolved. An **Authoritative Locator** is absolute, Declared-Root-relative,
URI-like, or harness-supported and is unambiguous in the declared execution
context. A bare artifact name is insufficient unless the packet explicitly
declares its base.

For fresh-child intake and outgoing packet validation, required task and evidence artifacts are resolved only from packet content or declared locators.
Missing, unreadable, or ambiguous required input stops fresh-child intake or the
affected dispatch before substantive child work with `BLOCKED` or the role's
schema-compatible context-gap result naming the exact gap; it can never yield
`PASS` or `ACCEPT`. The producer and child must not broaden discovery into ambient home or temporary
directories, unrelated workspaces, caches, or harness state to
repair the packet gap. Normal repository inspection for fresh-child intake begins only after all required packet inputs resolve and remains within the declared and permitted paths.
A directly invoked dispatcher may inspect only its user-selected execution
context; before child dispatch, every outgoing packet locator must resolve within
its declared and permitted paths. These constraints govern packet resolution and do not gate ordinary direct invocation in the user-selected execution context.

Load-task and task-system hooks may only enrich or validate supplied location
context. Delegation lifecycle hooks may enrich or validate a packet while retaining
their existing host-owned result handling. No hook can reconstruct a location
already known to the supervisor or make an otherwise non-self-locating dispatch
valid. Static structural and rendering tests verify that this contract is present
and survives adapter mappings; they do not prove model compliance or provide
runtime transport or filesystem enforcement.

Supervisors carry cumulative phase counters for mutating attempts, substantive
review rejections, and infrastructure failures through rebriefs and fresh
sessions. A dispatch counts as mutating when it edits the workspace or returns an
implementation; infrastructure failures before mutation are recorded separately
and do not consume the mutation budget. A blocked phase is eligible for redispatch only when new scope,
authority, specification, or environment evidence resolves its blocker; a
validated `scope_blocker` terminates the local repair loop.

After a phase's second substantive code or specification rejection, supervision
requires a fresh diagnostic before another mutation. It identifies the violated
DoD or invariant, relevant producer-to-consumer path, earliest shared enforcement
boundary, smallest root-cause fix, and regression that fails without it. The fix
prefers one shared guard or type constraint over denylist growth, sibling patches,
new abstractions, or refactoring. Recovery caps and mandatory gates remain in
force.

Reviewer rejections are grounded records. Each finding classifies itself as
`new`, `repeat`, or `scope_blocker` and names the breached DoD item,
specification clause, mandatory gate, or invariant plus verifiable source or
command evidence. Ungrounded findings cannot produce rejection. Reviewer findings
gate exclusively on code-level proof (diff quality, static types, security invariants,
mental mutation test); rejections citing absent dynamic evidence (runtime execution,
persistence checks, browser screenshots, or deployment validation) are out-of-authority
and filtered by the supervisor to QA delegation. QA Runner executes dynamic regression,
runtime, persistence, payload, and visual checks; accessibility auditing (WCAG 2.2) is
executed strictly when verifying end-user UI surfaces, evaluating to NOT_APPLICABLE for
backend, CLI, or library code.

The Deploy Supervisor coordinates post-merge deployment, database migration verification,
live endpoint smoke testing, and empirical release evidence collection. Deployments are
gated actions executed only under explicit human operator authorization, generating an
auditable RELEASE_EVIDENCE.md artifact.

## Supervisor Ledger Hook Contract & Curation Firewall

Canonical agents remain decoupled from host persistence engines, local files, and databases.
Storage for execution history is resolved exclusively through the declarative `<agent-hooks:invoke:record-ledger>`
hook declared in supervisor frontmatter. Canonical workers (`implementor`, `code-reviewer`, `planner`)
must never write to or consult local ledger files or storage engines.

The architecture enforces a Two-Tier Ledger Model:
- **Macro-Ledger (`plan-supervisor`):** Tracks phase-level state transitions across the plan DAG.
  Emits structured events containing `tier: "macro"`, `plan_id`, `phase_id`, `objective`, `summary`,
  `dependencies`, `status: PENDING|IN_PROGRESS|DONE|BLOCKED`, `revision`, `timestamp`, and `blockers`.
- **Micro-Ledger (`loop-supervisor`):** Tracks iteration-level transitions within an atomic task.
  Emits structured events containing `tier: "micro"`, `task_id`, `iteration`, `phase`, `status: PASS|FAIL|BLOCKED`,
  `mutation_count`, `review_rejections`, structured `findings` (id, classification, severity, breached_contract,
  evidence path:line, required_change), `remediation_targets`, and `timestamp`.

Supervisors act as an unbiased **Curation Firewall** across child dispatches:
- **Implementor Dispatches:** Supervisors forward only objective finding definitions (`F1: lease boundary equality in path:line`)
  and failing test gates from the ledger, stripping out subjective reviewer commentary, rhetorical critiques, or adversarial debate.
- **Reviewer Dispatches:** Supervisors forward only the remediation diff and the specific objective criteria from the ledger
  ("Verify whether finding F1 is resolved, without regressions"). Supervisors strictly suppress implementor rationalizations,
  apologies, or explanations that would soften adversarial review or prompt iterative goalpost-moving.
- **QA Runner Dispatches:** Supervisors forward strictly original DoD, test commands, and workspace changes, filtering out
  subjective code-quality opinions or developer commentary.
- **Expert Debugger Dispatches:** Supervisors forward strictly objective failing gate/test logs, breached contracts, and diffs,
  filtering out conversational histories.

Canonical bodies declare registered hooks with `<agent-hooks:list-available>` and
`<agent-hooks:invoke:<event>>` placeholders. During install or sync, the CLI
resolves each registered event once from host-global `~/.agent-hooks/`; Markdown
precedes an executable script. Deterministic hook events include `load-task`, `pre-plan`,
`classify`, `label`, `decompose`, `post-plan`, `record-ledger`, `pre-deploy`, and `post-deploy`.
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
shell profiles.

`AGF_INSTALL_DIR` is a bootstrap setting for the executable location; the CLI
does not expose a no-op self-install flag.
