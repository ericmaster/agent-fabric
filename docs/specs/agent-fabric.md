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
requires a root-cause diagnostic before another mutation. Reuse a finding's recorded diagnosis;
the parent consumes the child's diagnosis.
It identifies the violated
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
must never write to or consult local ledger files or storage engines. They may
consume objective task evidence and prior findings supplied by their supervisor.

### Continuity and checkpoints

The loop supervisor is the sole recovery owner for an atomic task. The plan
supervisor preserves the DAG and resumes that phase supervisor's recorded
session. Pass the recorded child continuation ID to the harness resume mechanism
on every later dispatch for that phase or role. Open a new session only when new resolving authority arrives, the approved scope identity changes, or the recorded continuation is unavailable.
Blocked status, missing child output, idle children, repeated gates, and context
length resume the recorded session or stay blocked. Initial reviewers are
independent of the author; their continuation receives the original contract,
objective findings and revision delta, never the author's conversation. Resume
also requires matching task, role, and execution root, plus refreshed
workspace/evidence state. Fresh sessions never reset task counters.
Reuse a finding's recorded diagnosis; a later session consumes that diagnosis.

The existing `record-ledger` hook supports `operation: load|record`. Supervisors
load the task's checkpoint after resolving the task and before dispatch; record
the event and full current checkpoint before dispatch and after each return.
The receipt carries a monotonically increasing `version`; records send its
`expected_version`. A stale writer reloads and reconciles, never overwrites.
An in-flight dispatch must be reconciled before a retry, even if no child session
ID was received. Missing persistence capability must be explicit: retain an
inline checkpoint for the current session, report that durable resume is
unavailable, and reconstruct from declared evidence before a fresh continuation.
An installed hook reporting a persistence error blocks further dispatch.

Checkpoint fields: `scope_id` (approved scope identity), `execution_root`,
`revision`, `worktree_state` (digest/locator including relevant uncommitted and
submodule state), `next_stage` (`select_phase|implementation|code_review|qa|
reconciliation|blocked|done`), `sessions` (role to session ID), `attempts`
(`mutating`, `review_rejections`, `infrastructure_failures`, `diagnostics`),
`findings`, `evidence`, `blockers`, and `in_flight` (null or dispatch identity and
role, with session ID when known). Macro checkpoints also carry `phase_states`
with each phase's status, counters, child session and evidence/checkpoint locator.
Macro top-level attempts are cumulative plan totals, not the newest phase's counters.
Evidence entries
identify exact command, execution root, input/runtime revision, result and full
log locator; a summary is not proof. Reentry starts at `next_stage`; repeat only
gates whose relevant inputs changed, or whose contract requires a fresh run.
A QA-only environment failure preserves valid code review and static gates.
Checkpoint metadata does not grant authority or certify evidence automatically.

`hooks/supervisor/record-ledger.py` is an opt-in, host/tool-neutral POSIX reference
hook (Python standard library). Hosts supply absolute `--root` (trusted artifact
directory) and `--execution-root`, and a JSON request on stdin. Identity is
`tier: micro` + `task_id`, or `tier: macro` + `plan_id`, scoped by execution root.
`load` returns checkpoint/version without creating absent state. `record` requires
`expected_version`, the event and full checkpoint; it atomically replaces one
JSON document containing the journal and checkpoint under an exclusive file
lock. Counters cannot decrease, even across scope changes. Concurrent stale
writes fail without changing state. Malformed/corrupt state fails closed; output
is one JSON receipt, exit 0 for success/absent and nonzero for error. This is
trusted per-task storage, not a filesystem sandbox or runtime dispatch engine.
Hosts own optional replicas; replica failures never erase local state. The
reference hook has no network, provider or task-system dependencies. Release
validation runs its Python unit tests alongside the Go test suite.

The architecture enforces a Two-Tier Ledger Model:
- **Macro-Ledger (`plan-supervisor`):** Tracks phase-level state transitions across the plan DAG.
  Emits structured events containing `tier: "macro"`, `plan_id`, `phase_id`, `objective`, `summary`,
  `dependencies`, `status: PENDING|IN_PROGRESS|DONE|BLOCKED`, `revision`, `timestamp`, and `blockers`.
- **Micro-Ledger (`loop-supervisor`):** Tracks iteration-level transitions within an atomic task.
  Emits structured events containing `tier: "micro"`, `task_id`, `iteration`, `phase`, `status: IN_PROGRESS|PASS|FAIL|BLOCKED`,
  `mutation_count`, `review_rejections`, structured `findings` (id, classification, severity, breached_contract,
  evidence path:line, required_change), `remediation_targets`, and `timestamp`.

Supervisors act as an unbiased **Curation Firewall** across child dispatches:
- **Implementor Dispatches:** Supervisors forward only objective finding definitions (`F1: lease boundary equality in path:line`)
  and failing test gates from the ledger, stripping out subjective reviewer commentary, rhetorical critiques, or adversarial debate.
- **Reviewer Dispatches:** Supervisors preserve the original contract and forward the remediation diff and specific objective criteria from the ledger
  ("Verify whether finding F1 is resolved, without regressions"). Supervisors strictly suppress implementor rationalizations,
  apologies, or explanations that would soften adversarial review or prompt iterative goalpost-moving.
- **QA Runner Dispatches:** Supervisors forward strictly original DoD, test commands, and workspace changes, filtering out
  subjective code-quality opinions or developer commentary.
- **Expert Debugger Dispatches:** Supervisors forward strictly objective failing gate/test logs, breached contracts, and diffs,
  filtering out conversational histories.

## Bug Intake Orchestrator

`bug-fixer` is a dispatcher-capable primary supervisor for reporters who are not
assumed to be technical. Every user-facing message, gate, verdict, and artifact
uses simple words. Approval gates are single yes/no questions. The agent mirrors
the reporter's language and does not invent details.

Intake captures one ticket per distinct defect using the portable report schema:
title, component, environment, severity (`urgent|high|medium|low`), steps to
reproduce, expected behavior, actual behavior, reporter, and project. An optional
`report-reviewer` pass may audit the report in a fresh context; exactly one
re-review pass is allowed after gap-filling.

Triage is `exec-ready` (single bounded fix, one component) vs `needs-plan`
(multi-component change or decomposable vertical slices), with a one-plain-sentence
rationale stored on the ticket.

Persistence uses the `persist-ticket` hook. The portable payload is the report
schema plus `triage{verdict, rationale}` and `tags`. When no hook is installed,
the ticket is written to `bugfix-tickets/UTC-ts-slug.md` under the current
execution root with self-generated id `bugfix-ts-slug`. The ticket locator is
carried into later dispatches as the child's task-system locator when the host
supports one.

Before children projection, bug-fixer writes a self-contained plain-language HTML
explanation at `bugfix-tickets/explanations/ticket-id.html` with exactly the
sections What is broken, The fix plan, and What happens next. One file per
ticket, rewritten in place on each plan revision.

Every dispatch requires an explicit yes. `exec-ready` dispatches `loop-supervisor`.
`needs-plan` dispatches `planner` (autonomous session; plan parent only, no
children), then after the user confirms projection invokes `decompose` with the
confirmed scope and rationale, then may dispatch `plan-supervisor`.
`report-reviewer` is a hidden reviewer subagent that returns
`{"verdict":"PASS|REVISE",...}` with findings in
`missing_detail|ambiguity|inconsistency`; a context gap is `REVISE` with a
critical finding naming the exact gap and can never yield `PASS`.
bug-fixer performs zero write-back after persistence; later ticket updates belong
to the dispatched supervisors.

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
shell profiles.

`AGF_INSTALL_DIR` is a bootstrap setting for the executable location; the CLI
does not expose a no-op self-install flag.
