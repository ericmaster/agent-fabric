---
title: "Spec: supervision support, ledger continuity, and QA contracts"
type: spec
status: active
covers: hooks/supervisor/support.py, hooks/supervisor/record-ledger.py, agents/qa-runner.md, tests/test_supervisor_support.py, tests/test_supervisor_ledger.py
last_checked: 2026-10-30
---

# Supervision Support, Ledger Continuity, and QA Contracts

Core CLI, schema, adapter, and manifest contracts live in [agent-fabric.md](agent-fabric.md).
Autonomous execution, bounded episodes, delivery ownership, and phase closure live in [supervision-and-delivery.md](supervision-and-delivery.md).

## Deterministic supervision support

`python3 <declared-fabric-root>/hooks/supervisor/support.py` consumes one JSON
request on stdin and emits one JSON receipt (0 success, 1 fail-closed error).
It is read-only: no ledger storage, dispatch, process signals, network, scheduler,
episode creation or model selection. Supervisors supply the checkpoint loaded
by `record-ledger`; record resulting transitions through that existing hook and
its expected-version guard. Missing helper support must be disclosed, not treated
as a completed preflight. Hosts may call this command from existing hooks.

- `operation: command-identity`, `pid`, `dispatch_id`, `execution_root` captures
  a Linux owned-command handle at launch: PID, `/proc` start ticks, boot ID and
  UID, scoped to the recorded dispatch/root. The caller must establish ownership
  before capture; observing an arbitrary PID does not grant ownership. Reconcile
  compares all identity fields, not PID alone; exited/zombie/reused processes are
  not live, unreadable or incomplete identity is a gap. Non-Linux hosts must
  provide their own verified identity support; never degrade to PID-only checks.
  Validate before probing: PID is a positive exact integer, UID a nonnegative
  exact integer (booleans excluded), start ticks a nonempty ASCII decimal string,
  boot ID a hyphenated hexadecimal UUID string. Missing/null/empty/ill-typed or
  malformed fields return `IDENTITY_GAP` with `may_dispatch: false`; a complete,
  valid identity differing from the observed process is not live (`UNAVAILABLE`
  when no live child/other command or terminal report remains).
- `operation: reconcile`, `checkpoint`, `session`, optional `report` inspects
  one current host session observation and `in_flight.commands` handles once.
  Session observations bind `dispatch_id`, `session_id`, `execution_root` and
  `state: live|busy|retry|idle|unavailable|unknown`, or native `status: {type: busy|retry|idle}`.
  Conflicting state/status is a gap. Matching live/busy/retry child or verified live
  command returns `WAIT/IN_PROGRESS` with the exact `pending_stage` (in-flight
  stage, or checkpoint next_stage). A matching terminal report (`PASS|FAIL|BLOCKED`,
  dispatch/session IDs, full locator) with idle/unavailable session returns
  `RECONCILE_REPORT`, never acceptance or redispatch. Idle/unavailable with no
  report/live command returns `UNAVAILABLE/DISPATCH_FAILURE`; unknown identity
  returns `IDENTITY_GAP`, no duplicate setup. No outcome grants dispatch authority
  (`may_dispatch: false`); supervisors reconcile effects and existing limits first.
  Observations/reports are trusted host inputs, not inferred from ambient history.
- `operation: fingerprint` requires `roots` (named absolute directories),
  `revision`, nonempty `inputs` (`{root,path}` regular-file locators), `checks`
  (`id`, exact `command`, object `context`, boolean `fresh_required`), and optional
  `receipts`. SHA-256 covers named roots, revision and sorted actual file bytes,
  including declared dirty files, specifications, config and QA inputs; each check
  additionally covers its command/runtime/fixture context. All declared inputs
  conservatively affect every assigned check. Ordering does not affect the digest.
  Caller owns completeness, including submodule/removed-file state and external
  runtime identity in context; HEAD alone is never adequate. Traversal, absolute
  relative locators, symlinks, nonregular, missing or unreadable inputs fail closed.
  A receipt (`check_id`, matching `digest`, `verified: true`, `status: PASS`,
  readable `locator: {root,path}` and matching SHA-256 `log_digest`) allows `REUSE`
  only without a fresh-run contract;
  otherwise return `RUN`. No helper output is PASS; absent/unrun/deferred evidence
  never satisfies mandatory gates. Verification flags are supervisor assertions,
  not certification by this helper. Receipt logs must be preserved unchanged.
- `operation: handoff`, `checkpoint`, `contract` produces a compact self-contained
  packet, excluding the event journal but retaining current findings/blockers,
  attempts, episodes, hard limits, continuation sessions, exact stage, owned handles,
  revision/digest, QA/unrun checks and evidence/checkpoint locators when present.
  Optional checkpoint fields `input_digest`, `inputs` (declared file locators),
  `checks` (assigned check definitions), `receipts` (verified receipt assertions),
  and `check_results` (fingerprint check decisions/digests/receipts) are retained
  unchanged when present. `worktree_state` does not replace `input_digest`.
  Together with contract roots and checkpoint revision, these support a fresh
  fingerprint decision after handoff; carrying a receipt does not certify it.
  Contract requires objective, non-goals, scope, authority, named roots, workspace
  ownership, authoritative inputs (inline or file locators), permitted source/evidence
  paths, commands, DoD, required evidence, rollback and unresolved-locator behavior.
  Permitted paths and required evidence use `root-name:relative-path`; future
  evidence destinations need not exist, but bare or escaping paths are refused.
  Required inputs must resolve; checkpoint execution root must be declared. No
  absent authority or unrun-check accounting may be invented. Preserve cumulative
  counters/episode metadata; never reset hard limits or silently change models.

Before broad QA, run a bounded readiness probe through the actual public command
and verify observable behavior, not just green helper tests. Retain failed probes
and unrun checks truthfully. Regression gate:
`python3 -m unittest discover -s tests -p 'test_supervisor_support.py'` plus ledger
tests and `go test ./...`.

## Discretionary QA Contract

QA is recommended, not mandatory. `loop-supervisor` owns the QA decision: run,
skip, reuse, or defer. A plan supervisor may recommend surfaces and retry deferred
QA at its discretion, without making QA dispatch a prerequisite for implementation.
Blanket procedural requirements for independent QA do not override this policy;
concrete product DoD, regression, safety and release requirements remain intact.

The minimal loop is implementor-owned relevant tests, independent code review,
then supervisor reconciliation. Dispatch QA only for assigned checks that add needed
evidence; run pre/post-QA hooks only when that dispatch happens. Supply the original
contract as reference and a bounded checklist, commands, revision/runtime, budget
and evidence root. QA reports only the assigned checks, not whole-plan acceptance.
It cannot add acceptance predicates, historical first-attempt-success requirements,
or implementation work. Existing scripts and helpers take precedence over new tooling.

Record `qa` with `decision`, `status` (`NOT_RUN|IN_PROGRESS|PASS|FAIL|SKIPPED|DEFERRED`),
`assigned_checks`, `unverified_checks`, `reason`, `evidence` and `retry_owner`.
A QA execution/setup/transport blocker becomes `DEFERRED`, with the exact cause,
unrun checks and receipt communicated to the superior supervisor (or direct user
when no parent exists). It does not stop remaining implementation, consume a product
rejection, invalidate passing review, or force immediate QA recovery. The parent
chooses whether/when to retry the same QA session with changed capability evidence.

Keep implementation readiness separate from acceptance. A phase report may retain
`IN_PROGRESS` for missing mandatory evidence while exposing `implementation_ready`
and deferred QA; subsequent implementation can use independently verified functional
prerequisites. An actual defect or unavailable functional prerequisite still blocks
dependent work. Never label skipped, deferred or running checks PASS. Final acceptance
requires evidence for substantive mandatory gates, obtained through authorized roles
or existing valid receipts, not necessarily through `qa-runner`. No independent QA
session is required for PASS, including integrated aggregate acceptance.

## QA Startup Contract

Before provisioning an environment or running acceptance flows, QA Runner reads
the assigned checks against the original DoD and identifies the project's stack and QA surfaces from
the packet's declared roots. It checks existing repository scripts, test/CI
configuration, runbooks, and supported stack-native tools, in that order, stopping
at the smallest suitable reproducible path. A documented command sequence or an
existing test command is sufficient; a universal launcher, browser, container,
authentication service, or database is not required when the task does not need it.

A suitable path makes startup, readiness, required fixtures/authentication,
verification, and task-owned cleanup reproducible where applicable. QA records
the selected commands/tool versions and runs a bounded readiness probe against
the intended revision/runtime before a full flow. Existing supported setup,
locked dependency installation, free-port selection, and disposable fixtures
remain autonomous within task authority. A failed probe does not prove a product
defect; ordinary bounded recovery follows the existing recipe.

If discovery and capability probes establish that no suitable path exists, or
that it needs tooling/configuration changes rather than routine execution,
QA returns `BLOCKED` with `QA_SETUP_REQUIRED` in the existing result
summary. Its evidence names the stack, inspected paths/tools, failed or missing
capability, affected DoD checks not run, and the smallest proposed reusable setup
with permitted files, dependencies, readiness/cleanup checks, and rollback.
It requests supervisor-owned setup or deferral; it does not engineer an ad-hoc
replacement during QA. Unaffected
checks may proceed with valid existing paths, but incomplete QA never yields PASS.

The receiving supervisor records deferred QA and communicates it upward while
continuing implementation. It chooses whether setup is worth pursuing. An
implementation-capable role owns tooling changes under existing task authority;
ask for approval only for scope, capability or policy not already authorized.
Legacy `QA_SETUP_APPROVAL_REQUIRED` reports receive the same deferred handling;
the marker alone is not evidence of a hard policy boundary. When setup is ready
and a supervisor elects to retry, resume the same QA session with reusable commands.

When a host/project supplies an isolated browser pool, QA uses an exclusive
process/profile lease, publishes the human watch URL, and connects a dedicated
client to the lease endpoint instead of a globally attached shared browser MCP.
It maintains the lease, stops on takeover/disconnect, rechecks ownership and
re-observes after reconnect, and releases in final cleanup. Uncertain side effects
are not blindly replayed. Persistent identity browsers require explicit login
scope. Host endpoints and checkout commands belong in the host skill/runbook,
not the portable canonical role.

Canonical definitions and adapter rendering checks preserve this instruction
contract; they do not implement a runtime approval lock or prove model compliance.

## Supervisor Ledger Hook Contract & Curation Firewall

### Outcome-first verification and bounded recovery

Design-led UI acceptance requires authentic external design/node/export provenance,
required copy/assets, viewport, and paired source/render evidence. QA distinguishes
fidelity from regression against implementation-derived screenshots; promoting the
latter cannot certify fidelity. Missing required source evidence precludes PASS.
Placeholder permission covers explicitly missing content only. Supervisors require
one representative source-matched page and publish its preview before replication,
then retain full final coverage. Changed acceptance invalidates affected gates;
children stop obsolete verification at a safe boundary before source mutation,
while unaffected evidence remains reusable.

The same failed prerequisite permits three materially distinct recovery attempts
across roles, followed by one bounded diagnostic (or reuse) and one evidence-backed
recovery per episode. Continued failure yields exhausted FAIL, or BLOCKED at the existing exact
external/policy boundary. Infrastructure failures count here independently of
substantive rejection counters. Rebriefs, new sessions and renamed scopes retain
this history. Renewed episodes require standing task authority and the distinct
strategy above. For QA-only prerequisites, defer instead of stopping implementation.
QA returns its recovery request after at most three attempts within its assigned budget;
supervisors own the final diagnostic/recovery allowance. At tool/child boundaries,
30 minutes without a newly verified criterion or restored prerequisite triggers a
user-visible no-progress update with the full evidence locator before continuing;
repeated unchanged notices are not progress.
This is a portable instruction contract, not a runtime timer, enforced spending cap,
or guarantee of model compliance. Required dispatch/return and transition ledger
writes remain; unchanged observations do not generate redundant checkpoint writes.

Canonical agents remain decoupled from host execution-ledger storage engines and databases.
Storage for loop/plan execution ledgers is resolved exclusively through the declarative `<agent-hooks:invoke:record-ledger>`
hook declared in those two supervisors' frontmatter. Bug-fixer keeps dispatch
receipts in its declared handoff artifact, separately from the ticket; deploy-supervisor
records release steps and child receipts in its declared release evidence. Both
reuse recorded child continuation IDs and reconcile in-flight effects on resume.
Canonical workers (`implementor`, `code-reviewer`, `planner`)
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

Planner records child continuation IDs, review and handoff receipts in planning
artifacts or supplied context, not execution ledgers. Subsequent same-role dispatches
pass the recorded ID; in-flight effects are reconciled and completed projection
receipts reused before any retry.

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

Keep one current packet per role/phase: objective, authority, scope/DoD, revision,
current delta and evidence locators, not copied reports or stacked amendments.
Load the full checkpoint once on intake/resume or version conflict; retain it in
the owning session and use record receipts thereafter. Status-only loads may set
`compact: true`: return status/version/path and, for existing state, next_stage,
revision and implementation_ready (false when absent), without a checkpoint.
Compact is a boolean allowed only on load; legacy load is unchanged. Validate the
same persisted state in both views. A compact receipt is not recovery/dispatch or
acceptance evidence: restore the full checkpoint before those decisions. Store
reports/history at durable locators, not inline in current evidence lists; preserve
all counters, handles, authority and unresolved conditions in the full checkpoint.

Checkpoint fields: `scope_id` (approved scope identity), `execution_root`,
`revision`, `worktree_state` (digest/locator including relevant uncommitted and
submodule state), `next_stage` (`select_phase|implementation|code_review|qa|
reconciliation|structural_recovery|aggregate_acceptance|integration|deployment|
verification|cleanup|blocked|done`), `sessions`
(role to session ID), `attempts`
(`mutating`, `review_rejections`, `infrastructure_failures`, `diagnostics`),
`findings`, `evidence`, `blockers`, and `in_flight` (null or dispatch identity and
role, with session ID when known). Macro checkpoints also carry `phase_states`
with each phase's status, counters, child session and evidence/checkpoint locator.
An overbroad phase additionally records `stable_revision`, `failed_revision`,
`split_rationale`, replacement scope IDs and dependencies, and aggregate-gate
status.
Macro top-level attempts are cumulative plan totals, not the newest phase's counters.
Evidence entries
identify exact command, execution root, input/runtime revision, result and full
log locator; a summary is not proof. Reentry starts at `next_stage`; repeat only
gates whose relevant inputs changed, or whose contract requires a fresh run.
A QA-only environment failure preserves valid code review and static gates.
New supervision checkpoints also retain `episodes`, `current_episode_id`, `qa`,
and `implementation_ready`; the reference hook preserves these additive fields.
Checkpoint metadata does not grant authority or certify evidence automatically.

`hooks/supervisor/record-ledger.py` is an opt-in, host/tool-neutral POSIX reference
hook (Python standard library). Hosts supply absolute `--root` (trusted artifact
directory) and `--execution-root`, and a JSON request on stdin. Identity is
`tier: micro` + `task_id`, or `tier: macro` + `plan_id`, scoped by execution root.
`load` returns checkpoint/version without creating absent state. `record` requires
`expected_version`, the event and full checkpoint; it atomically replaces one
JSON document containing the journal and checkpoint under an exclusive file
lock. Counters cannot decrease, even across scope changes. Concurrent stale
writes fail without changing state. Malformed/corrupt state fails closed without
writing; a rejected `next_stage` reports the complete accepted stage set listed
above without echoing the supplied value; output is one JSON receipt, exit 0 for
success/absent and nonzero for error. This is
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
- **QA Runner Dispatches:** Supervisors forward assigned checks, exact commands,
  workspace/runtime changes and original DoD as reference, filtering out subjective
  code-quality opinions or developer commentary. QA does not audit the entire plan.
- **Expert Debugger Dispatches:** Supervisors forward strictly objective failing gate/test logs, breached contracts, and diffs,
  filtering out conversational histories.

