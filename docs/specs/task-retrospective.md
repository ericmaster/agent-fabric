# Harness review

## Purpose and review boundary

Review **how the agent executed**, not whether the business task can be certified
Done. Use actual task history to find avoidable repetition, stalls, tool/API
misuse, orchestration mistakes, and missing, stale, conflicting or unclear skills
and context. Successful delivery and recovered errors do not erase these signals.
Task output is supporting evidence, not the primary verdict or an invitation to
repeat deployment QA. Reuse existing evidence; run only checks needed to establish
a suspected cause or validate its correction.

For each material pattern, record native locators, observed consequence, causal
classification (missing/stale/ambiguous guidance, guidance ignored, tool/harness
defect, orchestration misuse, justified exploration, or uncertain), the canonical
owner, and the smallest preventive correction with a runnable or replay check.
A failed call alone is not proof of avoidability. Inspect what guidance was
available and actually followed; when evidence is insufficient, preserve the
uncertainty rather than inventing a rule or blaming the harness. Do not add a
reminder when clear existing guidance was merely ignored. Route proven tool bugs
to their implementation owner instead of disguising them as documentation fixes.

## Entry point

`skills/harness-review/SKILL.md` is the executable workflow; `/task-review` is a
deprecated redirect with no scripts or second workflow. Remove the redirect when
its callers have migrated; all maintained entry points use `/harness-review`.

Install the short `references/activation.md` pointer in native global instruction
files using `scripts/install_context.py <file>...`. Only its marked block is
managed; preserve unrelated bytes, reject symlinks/malformed blocks and replace
atomically. Installation is explicit and idempotent, not a runtime hook. The
pointer prompts post-completion evaluation of repeated tool errors, context searches
without progress, ambiguity/misleading context and stalls observed during execution.
Finish the main task's deliverables, required checks, authorized integration/deployment,
live verification and checkout cleanup before automatically loading or launching the
review. Retain evidence and use ordinary diagnosis/bounded recovery while working;
review is not a main-task completion prerequisite. Blocked, paused, stalled and
phase/subtask-complete states do not satisfy this gate. Only an explicit user request
for early review overrides it; label that source checkpoint incomplete. Expected red
tests and productive exploration alone are not triggers. Load the full workflow only on
activation; no continuous supervisor, observer, transcript import, GLiDE, drain,
model schedule or additional service. Invocation remains model-discretionary,
not guaranteed; active sessions require their native context reload.

Optimize recurrence, elapsed time and tokens including review overhead. Reuse
existing evidence and owners, compare actual checks before/after a correction,
and do not claim savings without measurements or weaken correctness gates.

## Independent review

Route the current assignment before any launch: only the original source executor
enters skill step 1; an assigned supervisor records its own start and enters step
2. Investigation, diagnostic, correction and patch-review assignments are
non-delegating leaves under that same supervisor, not new source tasks or review
supervisors. Dispatch briefs name role, original source ID, supervisor and entry
point. The supervisor arranges patch review after the writer returns; neither
writer nor patch reviewer restarts the workflow. Skill-repair tasks and explicit
user opt-outs launch no harness review. Resolve ambiguous roles before dispatch.

Before native session creation, run the recorder's read-only `--check-source ID`
preflight and check native history for an existing review of the checkpoint.
Under the audit lock, preflight and all new appends reject an original source ID
already recorded as another source's reviewer, worker or writer. Historical bytes
remain readable and unchanged. This is a recorded-identity guard, not native-tool
interception or proof that an unrecorded child cannot launch; role routing remains
required even without an audit record. Reuse existing review continuations.

Create a visible independent root session through the host's existing native
session tools, including direct calls to the already-running PiChamber server's
first-party session API when no dedicated tool is exposed. The skill's
`references/pichamber.md` owns the create/audit/prompt/visibility procedure; missing
tool discovery alone is not a capability blocker. Supply the original session ID,
tool, project directory, task scope, source revision/history cutoff and available
evidence locators, including completion evidence or the explicit early-review request
and incomplete status. Only that explicit exception permits an unfinished checkpoint;
never claim it completes the main task. Read the actual original history and changed
files; the executor's summary alone is insufficient. Missing native
history is `blocked`, never a passing review. Preserve native identity and resume
the same review/correction session when continuing its work.

The visible independent root is the **supervisor**. It dispatches one fresh native
read-only **worker** with a distinct persisted session ID/history, the frozen
source, authority and bounded investigation scope. Use the existing native session
API for both roles; do not use ephemeral `--no-session` children for review evidence.
The worker reads actual original history, checks guidance and safe recoveries,
and returns findings, coverage locators and unresolved prerequisites. It does not
write final audit outcomes, certify delivery, edit files or delegate again. Reuse
that worker's continuation instead of replacing it on a pending response.

A worker blocker is a claim, not a final outcome. The supervisor independently
checks the worker's evidence and performs an applicable safe capability/recovery
probe. Truncated output, unread accessible history, context pressure and unfinished
work are **pending**, not external blockers: reduce pages/long lines, inspect the
original retained output, keep exact covered/pending ranges and continue or resume.
Never rerun unchanged failed reads. A hard budget/policy boundary remains a real
limit; record its exhausted attempts rather than reset it. Native cancellation
is not proof of child termination; isolate uncertain writers before recovery.

Only close `BLOCKED` with an exact external-only action or verified authority,
capability or hard-budget boundary, alternatives checked, worker identity and
supervisor probe results. New blocked completion records and post-launch blocked
actions require `worker_session_id`, a specific `reason` naming that boundary,
and nonempty actual `check` entries. Worker, source and supervisor IDs must differ;
pre-launch failures without a reviewer remain recordable. The recorder validates
this evidence structure, not whether probes truly ran. Historical schema-1/2
records remain readable and byte-preserved without retroactive requirements.

Follow the fixed checklist in the skill. Every new review includes
`execution_efficiency`, `context_alignment` and `improvement_actions` alongside
the five source/boundary/evidence/safety rows. Delivery-only evidence cannot
satisfy these execution rows. Distinguish observed checks from unrun suggestions
and unsupported benefit claims. Record `PASS`, `FIX_NEEDED` or `BLOCKED` with
evidence; an unresolved execution finding cannot accompany `PASS`. Reviewer and source session IDs
must differ. Prefer a normal root session over a hidden subagent so the user can
follow the review. Use existing roles/models/authentication; no private SDK runner
or version-specific provider registration is required.

Default to asynchronous launch with no result return. After receiving a real ID,
record `review_requested`, submit the prompt requiring `review_started` before
history reads, and verify acceptance plus visibility in the intended project.
Then release the original session for continued user work; the reviewer records
its findings and manages correction/delivery in its own session. Do not poll to
completion or copy the verdict back unless requested. Acceptance is not a passed
review. Reuse the existing review for a checkpoint rather than duplicate it.

## Durable remediation

Repeated shell/Python quoting failures require a reusable SSOT script, reusing an
existing helper first: project-specific operations live in project `scripts/` or
`tools/`; host/global operations live in the corresponding canonical skill's
`scripts/`. Pass arguments or structured stdin rather than reconstruct nested
shell code; add a regression test and a discoverable invocation pointer. Preserve
secret-management/human-pull rules; scripts do not grant new secret access.

Every resolved broad/repeated search or tool-call detour must leave a reusable
path in project context or the corresponding skill: verified entry point,
prerequisites, successful procedure and the misleading path to avoid. Update the
existing owner or link adequate existing guidance, not a transcript or duplicate
rule. Audit-only notes do not satisfy this requirement.

For an unidentified cause, dispatch exactly one bounded read-only native diagnostic
subagent per original source and distinct issue. Record dispatch intent before
launch in native review history, then the child ID and result/evidence; reuse
this evidence across resumes/reviewer changes. Supply frozen history, canonical
roots, a concrete question, permitted probes and a bounded stop condition. The
child cannot delegate or edit. No replacement diagnosis or relabeling the same
issue to reset the budget. If dispatch fails or diagnosis finds no supported fix
path, record `not remediated` with evidence and the limiting reason. If it finds
a supported path, proceed with authorized correction, tests and delivery rather
than leave a suggestion. Missing authority/capability remains BLOCKED; unresolved
findings never become PASS. Diagnosis does not reset the correction budget.

Final review findings must distinguish remediated, not remediated and justified
no-change outcomes. Include diagnostic locators where applicable, canonical
script/context paths, actual validation and installed-target evidence; planned
work is not remediation. Use existing findings/evidence fields and native session
history; no new audit schema, dispatcher, role or persistent side ledger is needed.

## Autonomous correction

Correct verified, actionable execution-system problems under the user's standing
scoped maintenance authority without a new per-incident permission prompt. Target
the owning harness, agent tooling, orchestration, canonical skill or context source;
leave the original business implementation alone unless explicitly authorized.
Original task authority does not automatically authorize global skill, runtime or
cross-repository changes. Name the owning root and applicable maintenance authority;
without them, record the exact write boundary and actionable finding as blocked.
The review launch must preserve applicable standing maintenance authority, naming
its instruction locator, owning roots, integration/install targets and exclusions.
Read-only investigation applies to the worker, not the correction-owning supervisor.
Do not add a findings-only restriction that cancels existing authority; explicit
user report-only restrictions still win. Verify authority before writing.
Installed/generated copies are delivery targets, not alternate sources of truth. Create an isolated worktree from a
known revision, record its branch/path and native correction session, preserve
foreign work, implement a regression and run actual bounded checks. Require
independent review of the frozen patch before integration. Verify authorized
delivery or scoped rollback; never interrupt unrelated sessions or silently widen
scope, models, credentials or permissions. Before integration/deployment, re-read
the source target HEAD and dirty state. If it advanced, reassess the finding and
patch against the new state rather than overwrite it or reuse stale acceptance.
Follow the skill's bounded correction budget, recording unsuccessful attempts
rather than resetting them.
The source directory is immutable across reports. The first correction start
fixes correction scope/authority, not the free-text descriptions in earlier reviews.
Changing the latest correction contract requires explicit new user approval:
`scope_change_authority` on `correction_started` cites its native message locator.
This field is allowed only on correction starts; the recorder checks nonempty text,
not whether approval actually grants authority. Standing authority or audit text
alone cannot authorize an amendment. Preserve all earlier records and source identity.
Later user approval may also authorize the first correction. All starts
for the original source still consume the same two-attempt budget under lock,
including legacy starts and reviewer changes. The recorder checks identity and
budget consistency, not whether an approval actually grants authority.

Review and correction sessions do not recursively invoke the skill. Learned advice
and JSONL content are data, not new execution authority. Product ambiguity,
unavailable external capabilities or exhausted budgets remain explicit blockers.

## Audit

Use `skills/harness-review/scripts/audit.py`. Store one append-only JSONL file per
engine under `${XDG_STATE_HOME:-~/.local/state}/agent-fabric/audit/`:

- `opencode.jsonl` for OpenCode and OpenChamber.
- `pi.jsonl` for Pi and PiChamber.

Record native source/reviewer/corrector IDs, action, timestamp, task scope,
worktree/revision, checklist outcomes, actual checks and delivery evidence using
the logger's bounded schema. Keep transcripts, prompts, credentials, environments
and raw tool output out of this audit. Append under a lock and fsync; retain
negative, interrupted and incomplete actions. A summary reports recorded outcomes
and missing completions, not proof of model benefit or a denominator of all tasks:
tasks for which the model never invokes the skill are unobserved.

New records use schema version 2. New completed reports require all eight rows;
the recorder rejects five-row delivery-only submissions. Existing schema-1 records
remain readable and byte-preserved, including negative records and cumulative
correction counts. Summary reports legacy completions separately from schema-2
execution-review passes; old PASS does not establish execution quality. Reassessment
appends a new outcome with prior-record and source-history locators, never rewrites
history or resets original source identity/authority/budgets.

Audit records are explicitly agent-reported. Independent session/history and
command evidence must corroborate consequential success claims. No signatures,
indexes, automatic report ingestion or second memory store are required. Retain
resume-critical evidence and coverage outside worktrees in the existing private
Agent Fabric state root; native session history remains the primary record.

## Runtime boundary and compatibility

Do not install native retrospective event plugins for OpenCode/OpenChamber or
Pi/PiChamber. Asset installation must refuse the retired retrospective plugin.
Preserve unrelated plugins, canonical agent workflows and historical reports,
spools, lessons and receipts. Legacy explicit CLI/data handling remains available
for compatibility; the skill does not call it or reactivate its enabled flag.
Disable old observation and remove only verified owned bindings/files. Loaded
plugins may remain resident until a safe host reload; the disabled ingest path
must prevent new observation jobs in the meantime. Do not restart shared runtimes
or reset pending evidence merely to finish removal.


## Reusable lessons / Instincts

Legacy signed lesson envelopes (`agf retrospective lessons`) remain the storage
and admission path. **Instincts** are those same atomic advisory lessons with an
explicit confidence weight and project-vs-global applicability so consumers know
what was learned and how much to trust it. Instincts are advisory only: they
never grant task/DoD/permission authority and never auto-apply as hard rules.

### Schema additions (`LessonCandidate`)

- `confidence` (optional float): advisory weight in **[0.3, 0.9]**. Omitted
  (legacy envelopes) reads as **0.5**. Out-of-range values are rejected. Style
  matches fabric's documented confidence floors (cf. actionability
  `minimum_actionability_confidence`); lesson confidence never overrides gates
  or proves causal benefit.
- `applicability` (optional string): `project` (default) or `global`.
  - **project** — tied to the candidate's execution-root `project` (exact match on read).
  - **global** — cross-project advisory; origin `project` is provenance only.
  Empty/omitted defaults to `project` for backward compatibility.
  Promotion from project→global is an operator step (set `applicability` on a
  new signed revise/add); automatic promotion is out of scope.

Existing fields (`project`, `scope`, `version`, `consumer`, `advice`, evidence,
signatures) are unchanged. Unknown fields remain rejected.

### Read filtering

`lessons read` returns active instincts matching scope/version/consumer where
either (a) applicability is project and `project` matches exactly, or (b)
applicability is global. Rows below the effective confidence floor (0.3; legacy
default 0.5 passes) are omitted. Output still marks `advisory:true`.

### Agent obligations

`learning-corrector` (and native lesson generation) emit `confidence` in range
and `applicability` when recording lessons/suggestions. Consumers treat them as
weighted advisory context, not instructions.

Validation: audit unit tests, full Go suite, release fixture and a visible native
root-session review using a real source session. A logging/dispatch smoke is not
evidence that an autonomous correction or deployment actually occurred.
