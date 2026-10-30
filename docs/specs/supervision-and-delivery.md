---
title: "Spec: supervision, autonomous execution, and delivery"
type: spec
status: active
covers: agents/plan-supervisor.md, agents/loop-supervisor.md, agents/deploy-supervisor.md, agents/bug-fixer.md, agents/planner.md, agents/plan-reviewer.md, internal/agent/supervision_policy_test.go
last_checked: 2026-10-30
---

# Supervision, Autonomous Execution, and Delivery

Core CLI, schema, adapter, and manifest contracts live in [agent-fabric.md](agent-fabric.md).
Delegation Packet and evaluation baseline rules live in [delegation-and-evaluation.md](delegation-and-evaluation.md);
ledger hooks, deterministic support, and QA contracts live in [supervision-ledger-and-qa.md](supervision-ledger-and-qa.md).

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

Bound each implementor dispatch by the remaining allowance before the next required
structural recovery decision. The child stops and returns its cumulative count and
evidence at that boundary; another mutation requires a recorded decision and an
eligible rebrief. This is an instruction contract, not runtime isolation.

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
non-mutating recovery path or a standing-authority new episode under the
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

Use the smallest existing functional smoke against the target runtime/producer
early, rather than stacking another mandatory check onto the task. Once its
functional prerequisite is verified, publish `implementation_ready` with the
candidate digest, evidence and pending checks; do not wait for exhaustive regression
or source review to start other implementation. Run worthwhile broader verification
in background on frozen inputs. The superior may advance provisionally in an
independent writable worktree/runtime, retaining prerequisite locators and pending
review/check handles. This does not mark the predecessor DONE, clear acceptance
dependencies or permit integration/deployment. A later defect stops affected
dependent work and is routed by the supervisor to the recorded owner; unrelated
isolated work continues. No new stage, scheduler, QA dispatch or acceptance gate.

Root-qualified artifact locators (for example `evidence:report.json`) resolve only
against the explicitly declared named root. They do not require another approval
or ambient discovery. Ambiguous roots, escaping paths and unreadable required
inputs still fail closed.

The initial episode has a soft budget of three mutations, with attempts four and
five allowed only for a materially distinct, reversible fix with a new failing
regression and an atomicity check. Five ends that episode; it is not a lifetime
cap imposed by this workflow. Explicit task, host, provider, spending, safety and
policy hard limits remain non-renewable by an ordinary continuation instruction.

After soft exhaustion, standing executable task authority permits the delivery owner
to evaluate a renewed episode without another user message. A renewed episode
allows at most two mutations by default. Record the original authority locator, new evidence
or diagnosis or authorized scope change, and a materially distinct causal strategy
before opening it. A new model, session, command spelling, or scope name is not a
new strategy. An automatic continuation is a wakeup, not additional authority or
evidence: use the existing task authority only after the strategy check. If there
is no distinct strategy, retain exhaustion and report the exact next decision once,
with a recommended recovery or explicitly deferred delivery; never ask for a ritual
renewal of an already authorized task. Further autonomous renewal requires a newly
verified acceptance criterion or restored prerequisite since the last renewal;
another hypothesis alone cannot sustain a chain of exhausted episodes.
Hard caps override standing authority.

Validation followed by a commit, push, or other mutation must use separate tool calls
with the validation result inspected first, or explicitly gate the mutation on every
validation's success, including pipeline failures. A later command cannot mask a
failed validation.

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
Automatic continuations cannot turn waiting or a hard cap into new work. A soft
episode ending transfers recovery to the delivery owner, not to the user's inbox.

## Delivery ownership and checkout closure

Plan, loop and deploy supervisors own delivery, not merely dispatch or a local PASS.
All three use mode `all` so native V2 delegation can reach them as well as root use.
Carry the original objective through applicable review, scoped commit, integration,
authorized deployment, live verification and cleanup. Record delivery stage, owner,
target and next action; an implementation report is not proof of a deployed result.
Inherited release approval remains valid for its named scope and target; honor
explicit repository/host confirmation rules without repeatedly requesting a granted
approval. Scope changes, destructive actions and safety boundaries retain their gates.

Inspect dirty status, diffs, provenance and active writers before mutation. Prefer
the current checkout with one writer when safe; use a worktree for actual concurrent
mutation, required isolation or an explicit request, not dirtiness alone. A host or
repository worktree mandate takes precedence. Review pre-existing work and preserve
unknown ownership; never auto-commit foreign changes or discard them to clean status.
Own integration back to the intended mainline and remove eligible task-owned temporary
resources. Successful delivery leaves task-owned source committed, integrated and the
owning checkout clean. Checkpoint incomplete work outside root main without claiming
acceptance, verify its recovery before restoring only owned hunks, and report any
retained foreign changes. Do not abandon dirty root main or erase unknown work.

## Native asynchronous orchestration

Portable shell hooks are explicit commands, not OpenCode plugin lifecycle hooks.
OpenCode 2 plugins use their installed generation's setup/domain API; do not assume
V1 hook names or deprecated idle events provide child completion. Verify registration
and version/capability compatibility against the running server, not only CLI version.
Treat busy and retry as in-flight. Idle means read the result, not successful delivery.
On a completed child result, reconcile the matching dispatch/session once, record the
transition and advance the exact next stage. Wait using native completion/notification
or supported wait APIs; do not create a polling agent or repeatedly prompt a live child.
Keep the parent responsible for the next action; never promise an unregistered callback.
On resume reconcile once; refresh only changed inputs. Use one compact current delta
and receipt locators instead of full transcripts or stacked handoffs. Native context
compaction retains objective, authority, stage, handles and unresolved findings; it is
not a reason to restart a role. Do not change pinned models merely to reduce tokens.

The reference ledger accepts integration, deployment, verification and cleanup stages.
An exact repeated event/checkpoint at the current version (ignoring observation
timestamp only) is a no-op (recorded=false),
not another progress event; stale version checks still apply. Reconciliation accepts
native session.status types busy/retry/idle as well as legacy live observations and
validates dispatch/session/root identity before using either form. Retry waits rather
than opening another child; conflicting or unknown state remains an identity gap.
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
continuation. Planner and plan-reviewer enforce proportionality to the seed
(clear architectural boundaries leaving coding mechanics to implementers,
lightweight default smoke/edge-case budgets unless statistical rigor is requested,
no unrequested schemas/DSL/fault-injection harnesses, pragmatic provider-alias
preflight, and `high`-severity `bloat` findings with subtraction bias against
reviewer scope creep) and omit routine counter/input-hash/closure-inventory
ceremony while preserving worktree/dirty-tree safety and credential redaction.
This spec is the authorship/testing SSOT; exported planner and
supervisor bodies carry concise critical safeguards inline, without requiring a
Fabric source checkout or duplicating the full policy. Optional detailed host
policy resolves only from declared packet roots; missing detail never waives the
inline safeguards. Policy tests verify instructions, not OS-mechanical enforcement.

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

