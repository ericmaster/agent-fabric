---
title: "Spec: delegation, evaluation baseline, and pre-review evidence"
type: spec
status: active
covers: agents/, internal/agent/model_test.go, internal/agent/eval_dod_test.go
last_checked: 2026-10-30
---

# Delegation, Evaluation Baseline, and Pre-Review Evidence

Core CLI, schema, adapter, and manifest contracts live in [agent-fabric.md](agent-fabric.md).

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

## Pre-implementation evaluation baseline

Every phase or atomic task requires a nonempty evaluation baseline before the
first implementation mutation. Each criterion declares a stable criterion ID,
observable pass/fail predicate with expected result, exact command or explicit
reproducible inspection procedure with expected observation, and required evidence
type and location. A command-free criterion still requires reproducible inspection.
Ordinary setup mechanics may be discovered within authorized roots; they cannot
invent or weaken acceptance criteria after implementation.

Identify the baseline by an inspectable immutable version/digest and authoritative
locator or inline snapshot. Capture it before the first implementation mutation;
carry that identity and criteria in implementation packets and final reconciliation.
Use existing packet/report/evidence patterns, not a new storage engine or persisted
ledger schema. Results map each criterion ID to actual outcome, executed command
or inspection procedure, inspectable evidence locator, and evaluated candidate
revision/digest. A command name or implementor assertion alone is not execution
evidence.

- Planner declares the baseline during design, before review, projection or dispatch
  (stable criterion IDs, observable pass/fail commands/procedures, and private evidence
  paths, without requiring `baseline.json` input pre-hashing or cryptographic audit
  trails for routine dev tasks).
- Plan-supervisor validates baseline completeness before phase dispatch and
  independently reconciles every criterion before phase PASS.
- Loop-supervisor validates before initial, retry and remediation implementation
  dispatch and independently reconciles every criterion before final PASS.
- Implementor validates before editing and reports results against the supplied
  baseline; missing criteria produce a precise gap, not manufactured criteria.
- Direct atomic invocations also establish a baseline from authoritative task inputs
  before mutation; a coherent atomic task does not require a parent planner.

Missing, empty, ambiguous or unresolvable criteria block implementation dispatch
and edits, never PASS. Supervisors may repair packets only from authoritative
predeclared inputs; unresolved product intent follows existing escalation rules.
Never derive acceptance silently from a finished patch. Changed criteria require
an explicit revised baseline, traceable reason and authority, and applicable plan
re-review before further implementation. Retain original criteria and change
history, invalidate affected evidence, and forbid retroactive weakening to turn a
failure into PASS. Retry preserves baseline identity and cumulative budgets.

Acceptance requires every declared criterion and existing mandatory gate to pass
with inspectable evidence tied to the evaluated candidate. Independently validate
evidence existence, content and candidate freshness, not merely matching report
text. Absent, failed, stale, contradictory, skipped, deferred or unrun mandatory
evidence cannot PASS. Discretionary QA remains discretionary; `implementation_ready`
is provisional readiness, not acceptance, and neither bypasses mandatory evidence.
The baseline establishes predicates up front; it does not require full failing
product tests before every docs/prompt edit or that every new test fail before
every edit. Static contract/rendering tests do not prove live model compliance
or provide runtime transport enforcement.

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

