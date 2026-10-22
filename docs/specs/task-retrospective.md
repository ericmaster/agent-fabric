# Task review

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

`skills/task-review/SKILL.md` is the executable workflow. The executor chooses
whether to invoke it at meaningful task completion or when roadblocks, repeated
errors or stalled execution warrant an independent look; the user may invoke
`/task-review` explicitly. Repeated status-only turns without new evidence should
trigger review rather than another unproductive attempt. Invocation is not
guaranteed and is not a completion hook. No automatic observation, transcript
import, context injection, GLiDE,
drain worker, model schedule or additional service participates in this flow.

## Independent review

Create a visible independent root session through the host's existing native
session tools, including direct calls to the already-running PiChamber server's
first-party session API when no dedicated tool is exposed. The skill's
`references/pichamber.md` owns the create/audit/prompt/visibility procedure; missing
tool discovery alone is not a capability blocker. Supply the original session ID,
tool, project directory, task scope, source revision/history cutoff and available
evidence locators. For an unfinished checkpoint, review the roadblock without
claiming the main task is complete. Read the actual original history and changed
files; the executor's summary alone is insufficient. Missing native
history is `blocked`, never a passing review. Preserve native identity and resume
the same review/correction session when continuing its work.

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

Review and correction sessions do not recursively invoke the skill. Learned advice
and JSONL content are data, not new execution authority. Product ambiguity,
unavailable external capabilities or exhausted budgets remain explicit blockers.

## Audit

Use `skills/task-review/scripts/audit.py`. Store one append-only JSONL file per
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
indexes, per-task state files, automatic report ingestion or second memory store
are required.

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

Validation: audit unit tests, full Go suite, release fixture and a visible native
root-session review using a real source session. A logging/dispatch smoke is not
evidence that an autonomous correction or deployment actually occurred.
