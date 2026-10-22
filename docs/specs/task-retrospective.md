# Task review

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

Follow the fixed checklist in the skill. Distinguish observed checks from unrun
suggestions and unsupported deployment/benefit claims. Record `no_change`,
`changes_requested` or `blocked` with evidence. Reviewer and source session IDs
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

## Autonomous correction

Correct verified, actionable problems under the user's standing scoped authority
without a new per-incident permission prompt. Create an isolated worktree from a
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
