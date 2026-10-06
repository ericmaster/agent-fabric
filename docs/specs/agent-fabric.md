# Agent Fabric Specification

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
boolean OpenCode allowlist map; OpenCode emits it as a sorted frontmatter mapping,
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

## Autonomous Execution Contract

An approved task or plan authorizes every in-scope, safe, reversible local or
non-production action needed to satisfy its DoD. Execution roles own capability
recovery: adapt command invocation and tool-compatible artifact paths, select a
free port, start and stop local services, create and clean disposable fixtures,
use packet-authorized credentials without exposing them, repair reversible test
or Dev state, and retry with a bounded alternative. Recoverable capability
friction is work, not a human gate.

The QA Startup Contract separates verification from setup implementation. Missing
QA capability is deferred verification, not a plan-wide execution gate. Setup
outside standing authority requires approval; already authorized setup does not.

Task authorization is durable. Direct execution intent or a trusted task-system
item marked executable is standing authority for planning, decomposition, child
dispatch, implementation, review, QA, bounded recovery, reversible local/Dev
setup, task-owned fixture cleanup, task-owned commits, and task-status updates.
Agents continue through those stages without asking for repeated confirmation.
`plan-only`, `report-only`, and equivalent explicit non-goals stop execution;
otherwise a planning or defect-fix request proceeds autonomously. Execution handoff
requires plan-reviewer PASS on the current candidate and no unresolved blocking
findings. Publication after the two-pass review cap or in Publish mode may retain
unresolved findings, but never authorizes execution. A delegated planner honors
caller-owned projection and dispatch; bug-fixer owns its needs-plan handoff.
An executable
release task with a named target environment, deployment DoD, rollback, and
traceable authority from its current request or approved parent is durable
authorization for its complete release sequence; the deploy supervisor verifies
that chain once and does not ask again between steps. An agent may execute such a
task but may not manufacture a new release objective outside its authorized
parent scope.

Budget efficiency and functional completion govern execution. Start with the
least expensive configured role/model and smallest relevant test or evidence set
that can prove the next decision. Reuse unchanged evidence and existing sessions;
do not duplicate discovery, reviews, tests, or failed attempts. Escalate model
capability, test breadth, or delegation count only after objective evidence shows
the cheaper path is inadequate. Mandatory final gates and DoD are never reduced
for cost.

Safety measures are selected autonomously and proportionally to blast radius,
reversibility, and expected loss. For stateful, production, destructive, or
irreversible work, the supervisor verifies the exact target and scope, creates the
best available checkpoint or backup, uses the smallest viable canary or batch,
defines success and rollback signals, executes, validates, and attempts rollback
or predeclared compensating recovery on failure. Restoration must be verified;
irreversible effects never imply guaranteed rollback. It chooses the least costly controls that bound the actual risk
instead of requesting approval. Secret values remain process-memory-only and must
never be disclosed or persisted.

Every supervisor-profile agent is the resolver of blockers, not a blocked
participant. A child permission boundary, refusal, malformed result, missing tool,
failed command, absent documentation, unsuitable fixture, unavailable local
service/image, workspace problem, or reversible Dev-state failure remains an
internal recovery state. The supervisor diagnoses it, repairs it directly when
authorized, delegates it to a role with the required capability, provisions a
replacement, or resumes with a materially distinct path. It never relays a child
`BLOCKED` unchanged. A supervisor may surface `BLOCKED` only for a verified exact
action that requires unavailable external capability or credentials, unresolved
product intent, or an explicit non-overridable policy/scope boundary. Production,
destructive, or irreversible mechanics are risk classes to control autonomously,
not approval categories.

An authorized secret-loader-to-client handoff is secure when the value remains in
process memory and passes directly from an environment variable to the local API
or browser input. The command, transcript, logs, URLs, screenshots, and artifacts
must not contain the value. A user-pasted credential or secret-bearing temporary
file is neither required nor permitted when this direct handoff is available.

Evidence remains valid at a packet-permitted tool-owned locator when a browser or
other tool cannot write to the preferred evidence root. The role records the
actual locator and continues; it does not weaken the evidence requirement.

`BLOCKED` is reserved for a requirement that genuinely needs unavailable external
capability or credentials, an
unresolved product decision, or an explicit non-overridable policy/scope boundary. A
supervisor repairs child packets and environments with authority it already has,
then resumes the same child without requesting another user instruction. New
resolving evidence may be produced by that autonomous recovery; it does not need
to arrive in a new user message.

A `BLOCKED` result is a claim to verify, not a verdict to relay. It identifies the
exact action only an external actor can perform and includes executed capability
probes plus materially distinct recovery attempts. Missing documentation, a
preferred tool/path, a pre-existing fixture, a local image, or ready-made test
state does not prove external dependence. The execution role inspects declared
repository guidance, performs ordinary setup, builds or starts safe local/Dev
dependencies, and provisions a disposable workspace, bot, account, or fixture
when the existing one is unsuitable. “Not supplied”, “not documented”, and “would
require setup” are insufficient blocker evidence.

The context firewall protects authoritative task and design inputs that only the
producer can locate or decide. A command, repository procedure, or fixture setup
discoverable inside permitted declared roots is ordinary operational work, not a
context gap and not a reason to demand command-by-command user instruction.

Implementation and verification mechanics are part of approved execution unless
the current task explicitly excludes them; they do not need command-by-command
enumeration. Supervisors autonomously choose proportional controls for
authentication, network, path, production, destructive, and irreversible work
within the authorized objective. They never disclose secrets or silently expand
the product objective.

Supervisors carry cumulative phase counters for mutating attempts, substantive
review rejections, and infrastructure failures through rebriefs and fresh
sessions. A dispatch counts as mutating when it edits the workspace or returns an
implementation; infrastructure failures before mutation are recorded separately
and do not consume the mutation budget. A blocked phase is eligible for redispatch only when new scope,
authority, specification, or environment evidence resolves its blocker; a
supervisor-verified external or policy `scope_blocker` terminates the local repair
loop. A child's context gap or environment `BLOCKED` is an internal recovery claim,
not proof of external dependence.

Budget exhaustion is not itself an external blocker. After each root-cause
diagnostic, and mandatorily before a fourth mutation, the atomic supervisor tests
whether the phase is structurally overbroad. A phase requires decomposition when
the remaining findings span multiple independently testable producer-to-consumer
paths or state machines, need disjoint acceptance surfaces, or cannot be repaired
at one shared enforcement boundary without combining unrelated mutations. A
repeated finding does not require decomposition when one coherent root-cause fix
still closes the path.

For an overbroad phase, the supervisor freezes the latest failed checkpoint as
diagnostic history, identifies the last revision with verified prerequisite
evidence, and proposes complete vertical replacement slices. Each slice owns one
producer-to-consumer path, explicit DoD and gates, permitted paths, and dependency
edges. The failed phase retains its counters and becomes a non-mutating aggregate
acceptance gate over the original DoD. Every original DoD item belongs to at least
one replacement slice; the aggregate gate verifies only their union and owns no
unique implementation requirement. Each replacement slice receives a
deterministic scope identity and workspace from the stable revision. Its counters
inherit failed parent attempts attributable to that path; only a previously
untouched path starts at zero. One structural split is allowed per lineage, and an
equivalent slice cannot be recreated to reset counters. Macro totals and the
failed phase's history never decrease. Failed-branch changes
are reference material only and may be selectively reapplied after inspection,
never inherited wholesale.

The plan supervisor owns composition into one recorded integration revision.
It also owns replacement-DAG acceptance: obtain independent plan-reviewer PASS
before materialization or execution, including direct loop-supervisor split handoffs.
Use at most two passes, resume the reviewer, and revise the same deterministic
slices without resetting counters; unresolved rejection returns terminal FAIL.
An original plan's PASS does not cover a changed replacement DAG.
Dependent replacements consume verified predecessor changes on top of the stable
base. Conflicts return to the owning slice's existing session and counters;
invalidated gates rerun. Aggregate acceptance verifies the integrated revision
containing all accepted outputs, never only isolated PASS reports.

When a parent plan exists, `loop-supervisor` returns `recovery.mode: split` and a
validated split proposal to `plan-supervisor`. The plan supervisor invokes the
`decompose` hook, materializes replacement slices and dependencies, blocks later
phases on the aggregate gate, and continues without human confirmation. On direct
atomic invocation, loop-supervisor hands the replacement DAG to `plan-supervisor`
and stops its failed-scope loop; if dispatch is unavailable, it returns the split
proposal as `FAIL`, not `BLOCKED`. The plan supervisor reuses its rendered
`decompose` executor during recovery; without an installed hook it updates the
inline macro DAG and discloses missing durable task-system materialization. A
replacement that appears overbroad invalidates the partition and causes a
non-mutating revision of the same deterministic slices, never a recursive split.
Only inability to preserve the
original DoD without a product decision or policy expansion may escalate the
split. At the mutation cap, evaluate structural recovery before external escalation.
When the task remains atomic and no permitted remediation remains, return terminal
`FAIL` with `recovery.mode: exhausted`, counters, failed gates, and rejected-split
evidence. The parent must not redispatch that exhausted episode without a valid
non-mutating recovery path or an explicitly authorized new episode under the
bounded episode contract below. Hard limits remain in force.
Retryable FAIL uses `retry`; overbroad FAIL uses `split`; verified external BLOCKED
uses `external_block`; PASS uses `none`. Exhaustion alone never means BLOCKED.

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
and filtered by the supervisor to its discretionary QA decision. QA Runner executes assigned dynamic regression,
runtime, persistence, payload, and visual checks; accessibility auditing (WCAG 2.2) is
executed strictly when verifying end-user UI surfaces, evaluating to NOT_APPLICABLE for
backend, CLI, or library code.

## External pre-review evidence

A supervisor may dispatch the optional pre-delegate hook to collect external
pre-review evidence for an authorized change. This contract governs that
evidence on both sides of the seam:

- Portability: canonical bodies name only an "external pre-review evidence
  source"; provider, endpoint, key, model and question details live in the host
  hook, never in portable agent bodies.
- Advisory only: typed external judgments (labels, probabilities, confidence)
  are attention-routing leads. They can neither certify syntax, types, or
  security nor verify DoD completion, and they can never directly produce
  `ACCEPT`.
- Provenance: the packet carries the pinned diff/base/head identity, original
  task/spec/DoD, the question rubric plus model and exact response receipt, and
  referenced exact-command evidence.
- Reuse safety: inspectable exact-command evidence is reused only for unchanged
  relevant revisions; missing, stale, low-confidence, or unverifiable external
  evidence triggers the reviewer's local checks. Mandatory static, security,
  and QA gates are never waived by an external rating.
- Fallthrough: when the hook is absent, unavailable, or returns unavailability,
  the independent reviewer runs the unchanged normal path; missing
  authoritative task/spec/diff inputs remain a task-context gap, never an
  engine outage.

The Deploy Supervisor coordinates post-merge deployment, database migration verification,
live endpoint smoke testing, and empirical release evidence collection. A scoped executable
release task is durable authority; the supervisor autonomously applies proportional
checkpoint, canary, verification, and rollback controls and generates an auditable
RELEASE_EVIDENCE.md artifact.
Release intake and outgoing child packets follow the same delegation contract.
Continuations reconcile actual target effects before repeating release steps.
Parity is against the authorized target revision rather than a moving HEAD.
Release status is `DEPLOYED|VERIFIED|FAILED|ROLLED_BACK|BLOCKED`; unrun execution
and migration checks use `NOT_RUN`. Failed restoration is `FAILED`, never VERIFIED.

## Bounded episodes, waiting, and reduced delivery

Resume-critical checkpoints, candidate trees and acceptance evidence must live in
a declared durable private root outside reboot-cleared temporary storage. `/tmp`
is scratch only, never the sole authority for a continuation or a reviewed diff.
Packets carry the durable root and receipt locators before dispatch. After a
crash, reconcile surviving source hashes, receipts and command boot/start identity;
missing temporary evidence remains unavailable, never reconstructed as PASS or
used to reset budgets. Refresh only invalidated checks, not unchanged completed
ones. Preserve private permissions and unrelated work during relocation.

Before an expensive check, record the user-facing deliverable and whether the
current phase changes the public preview or only a private draft. A mismatch with
the user's delivery goal is surfaced to the superior once, not hidden behind
setup or QA progress. Run a representative dependency/bootstrap smoke at the actual
test-fixture seam; full-application health does not establish fixture readiness.

An owned long-running command is not a serial gate on independent source review.
Review frozen inputs while QA runs, without mutating those inputs or weakening
final acceptance. Track each continuation independently; reconcile a completed
focused receipt even when an aggregate remains running. Publish a nonterminal
progress receipt with the pending stage, owned handle, material output delta and
full-log locator. Provide bounded progress updates through the available monitor
or the next user interaction; never promise only a terminal callback that the
harness cannot deliver. Do not repeatedly poll, relaunch a live command or describe
unchanged messages as progress.

Root-qualified artifact locators (for example `evidence:report.json`) resolve only
against the explicitly declared named root. They do not require another approval
or ambient discovery. Ambiguous roots, escaping paths and unreadable required
inputs still fail closed.

The initial episode has a soft budget of three mutations, with attempts four and
five allowed only for a materially distinct, reversible fix with a new failing
regression and an atomicity check. Five ends that episode; it is not a lifetime
cap imposed by this workflow. Explicit task, host, provider, spending, safety and
policy hard limits remain non-renewable by an ordinary continuation instruction.

After exhaustion, a direct explicit user instruction to continue permits the
supervisor to evaluate one new episode, not automatically mutate. A renewed episode
allows at most two mutations by default. Record the authority locator, new evidence
or diagnosis or authorized scope change, and a materially distinct causal strategy
before opening it. A new model, session, command spelling, or scope name is not a
new strategy. Automatic goal continuations do not authorize new episodes. If there
is no distinct strategy, retain exhaustion and report the exact next decision once.

Before every mutation, record a compact mutation brief: hypothesis, difference from
prior attempts, evidence, failing regression, budget and rollback. For initial work,
the regression is the acceptance test that fails without the change. Reuse recorded
diagnoses; a repeated failure requires diagnosis before another mutation. Two
substantive rejections within an episode trigger diagnosis, not another equivalent
attempt. Close an episode early when no new evidence supports recovery.

Keep `attempts` as monotone task/plan history. Checkpoints also retain `episodes`
(append-only closed history plus the current entry) and `current_episode_id`.
Each entry carries `id`, `authority`, `strategy`, `evidence`, `budget`,
`baseline_attempts`, `attempts` (episode-local counters), and `status`.
Opening a new entry never changes the task/scope identity or decrements historical
counters. Older checkpoints without episodes enter the initial episode with their
existing counters, not a fresh allowance. Splits inherit attributable history;
renaming or splitting cannot bypass a hard cap. Macro checkpoints retain the
phase episode metadata and cumulative plan totals.

An active child or command is `IN_PROGRESS`, not a failed gate. Preserve the pending
stage and `in_flight`; wait for its result with the native completion/wait mechanism
when available. Do not dispatch an equivalent child, repeatedly audit unchanged
state, consume mutation/rejection budget, or claim progress merely for waiting.
Automatic continuations cannot turn waiting or terminal exhaustion into new work.
Record actual dispatch/return/state changes; send one no-progress notice with the
full evidence locator, then update only on changed evidence or a required deadline.
These are portable instructions, not a runtime Goal Mode watchdog.

On an authorized scope reduction, stop obsolete child work at a safe boundary,
reconcile uncertain effects and resource ownership, and deliver only the accepted
revision for the revised scope. Separate implemented, deferred and still-defective
behavior. A reduced delivery does not complete the original plan. Verify the target
revision and existing runtime evidence before deciding to build or deploy; a revision
already deployed and verified is reused, not rebuilt or redeployed.

## Phase closure and compact planning

Every phase closes in order: validate required checks on current accepted inputs,
independent source review, immediate scoped commit/integration under repository
workflow, verified compact durable receipt, then owned-resource cleanup. The
packet names the commit/integration owner; an implementor forbidden to commit
hands the reviewed candidate to that owner. Never end phase acceptance PASS with
valid uncommitted code. Blocked mandatory gates do not authorize a commit;
docs-only work records applicable checks, not invented test PASS. Integration
must contain the reviewed scope; changed inputs invalidate affected evidence.

The receipt lives in the declared durable private root and records acceptance
scope, current accepted input hash, source/reviewed/integration revisions, exact
commands and results (including unrun checks), independent review disposition,
decisions and rationale, important rejected alternatives, unresolved conditions,
failure history, cumulative counters, episodes and budgets/hard caps, continuation
handles, retained/deleted inventory, and each resource's ownership, consumer
closure and cleanup eligibility evidence. Verify summary completeness against
source evidence before removing any source-unique proof. Preserve existing
ledger/continuity policy; this contract adds no daemon, agent or runtime executor.

Successful COMPLETED accepted-scope raw logs may be removed or compacted only
after all consumer references are resolved and the summary is verified. Consumers
include review, receipt reuse (log digest/locator), continuation and external
retention requirements; retire or rebind references explicitly, never leave
dangling proof locators. Retain full raw logs for unresolved FAIL/BLOCKED/DEFERRED,
incomplete acceptance or required external retention. While unresolved, summaries
preserve failure history, counters/budgets and traceable full-error links; never
trim failures prematurely. IDLE is not command completion: reconcile active
handles and PID/start/boot identity, not just session status or PID alone.

Remove worktrees/branches only when task-owned, with no active handles or live
commands, a clean tree and integrated commit ancestry, or explicit authorized
discard of named content with backup/recoverable source where needed. Unknown
ownership, dirty or unmerged work is retained with the precise eligibility gap.
Restore temporary working changes only for proven task-owned named hunks/paths,
with recorded before hash and proof of no unrelated concurrent edits. Never use
blanket checkout, force worktree removal, reset/clean or broad deletion. Arbitrary
used/unassigned resources or absence of callers do not establish obsolescence.
Do not erase current resources, macro-ledgers, native fixtures or budgets. Cleanup
follows valid scoped commit/integration and the verified receipt, never precedes
them or deletes the only recovery source.

At plan readiness, put compact grilling decisions, rationale, important rejected
alternatives and unresolved conditions in plan detail (Context & Constraints,
outside parser-sensitive phase fields). Unresolved blocking conditions keep the
candidate unaccepted, not an executable plan. After verifying the complete summary,
retire task-owned temporary grilling questionnaires, diagrams and raw Q&A artifacts;
no full grill-session export or per-question archive is required. This retirement
does not cover operational failure logs, third-party materials, foreign artifacts
or evidence still needed by consumers. Preserve incomplete planning artifacts for
continuation. This spec is the authorship/testing SSOT; exported planner and
supervisor bodies carry concise critical safeguards inline, without requiring a
Fabric source checkout or duplicating the full policy. Optional detailed host
policy resolves only from declared packet roots; missing detail never waives the
inline safeguards. Policy tests verify instructions, not OS-mechanical enforcement.

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
  `state: live|idle|unavailable|unknown`. Matching live child or verified live
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
this history. Renewed episodes require the explicit authorization and distinct
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

Checkpoint fields: `scope_id` (approved scope identity), `execution_root`,
`revision`, `worktree_state` (digest/locator including relevant uncommitted and
submodule state), `next_stage` (`select_phase|implementation|code_review|qa|
reconciliation|structural_recovery|aggregate_acceptance|blocked|done`), `sessions`
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
- **QA Runner Dispatches:** Supervisors forward assigned checks, exact commands,
  workspace/runtime changes and original DoD as reference, filtering out subjective
  code-quality opinions or developer commentary. QA does not audit the entire plan.
- **Expert Debugger Dispatches:** Supervisors forward strictly objective failing gate/test logs, breached contracts, and diffs,
  filtering out conversational histories.

## Bug Intake Orchestrator

`bug-fixer` is a dispatcher-capable primary supervisor for reporters who are not
assumed to be technical. Every user-facing message, gate, verdict, and artifact
uses simple words. The defect report is standing execution authority unless it is
explicitly `report-only`; only irreducible reporter or product decisions are asked.
The agent mirrors the reporter's language and does not invent details.

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

`exec-ready` dispatches `loop-supervisor` under standing execution authority.
`needs-plan` dispatches `planner` (autonomous session; plan parent only), then
invokes `decompose` with validated scope and rationale and dispatches
`plan-supervisor` without repeated approval prompts.
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
