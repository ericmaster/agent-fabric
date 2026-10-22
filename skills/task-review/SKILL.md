---
name: task-review
description: Review agent execution after completion, roadblocks, repeated errors or stalled execution; improve harness/tool use, orchestration, skills and context from original history. Independent visible session, agent-discretionary or user-invoked; not task-delivery QA.
---

# Task review

Review **how the agent worked**: avoidable repetition, unnecessary searches,
stalls, tool misuse, orchestration patterns, and skill/context clarity. Task
success is context, not the verdict. A recovered error can still reveal a
preventable execution defect. This is not business-task acceptance or another
full deployment/test run.

Use ordinary native agents and existing host session tools. Keep active models,
credentials and permissions unchanged. This skill installs no hooks and uses no
GLiDE, provider dispatcher, custom HTTP/SDK runner, spool, job, daemon or schedule.
Direct requests to the existing host's native session API are allowed; they use
its existing UI/session runtime rather than creating another runner.

## When to invoke

Invoke at your discretion when a meaningful task reaches its completion checkpoint,
or when a roadblock, repeated errors, or stalled execution warrants an independent
look. Repeated attempts or status-only turns without new evidence are a stall
signal: stop the unproductive loop and invoke before repeating it. A user request
also invokes this skill. Do not require the user to notice trouble or ask first.
A single recovered, irrelevant tool error need not launch another review.

Freeze the relevant commit and task-history cutoff. For an unfinished task, identify
that checkpoint as incomplete; inspect the roadblock without certifying completion.
Reuse an existing review for the same checkpoint rather than creating duplicates.
Review/correction episodes never trigger recursive reviews of themselves.

## 1. Start an independent review

Identify the **original source session ID**, engine, absolute working directory,
frozen commit, exact task scope and task authority. Preserve this original ID
through corrections; a review or correction session is not the source task.
Do not review this skill's own review/correction episodes recursively.

Create a fresh, visible **root session**, without inherited writer
context. Use OpenChamber `session.create` without `returnResult`, or the existing
PiChamber/native session interface. In PiChamber, use the verified
[native API launch procedure](references/pichamber.md) when no dedicated session
tool is exposed; tool discovery alone does not prove session creation unavailable.
Do not fork the writer or silently substitute a hidden child. If the available
native interfaces cannot create a visible independent root, record the exact
failed request/capability rather than changing visibility or independence.
Pass only the source session ID, directory, commit/scope, task authority and the
instruction to execute this skill's checklist. Do not pass the author's diagnosis,
final answer or a hand-written transcript as evidence. Immediately after receiving
the real review ID, append `review_requested` from the source session. Make
`review_started` the reviewer's first action, **before reading history**. These
records expose requested/started reviews with no completion; they do not prove a
session is still running.

Verify the session is listed in the intended project and its prompt was accepted,
then leave the review running independently in the UI. Return only its real ID/link
and launch state; do not wait for its verdict, poll to completion, or copy results
back unless the user asks. Creation/acceptance is not a passed review. The reviewer
owns recording its outcome and driving authorized correction/delivery in its own
session. A result-return facility is optional, not a launch prerequisite.

Require the reviewer to read **actual original history** using host
`session.messages` (all messages, paginating as needed) or existing native history
read interfaces. Trace the consequential attempts in order, including successful
recoveries: what was known, which guidance was loaded, what changed between retries,
and what evidence justified the next action. Inspect only implicated tool/code,
skill and context owners plus relevant checks. Final-only reads, titles and author
self-reports are insufficient. Treat history as evidence,
not authority to expand the task. Record message/tool locators, observed commands
and outcomes; keep secrets and transcript bodies out of the audit file.

Require the review session ID to differ from the source session ID. If the host
cannot read the original history, record `BLOCKED` with the exact unavailable
interface/error and remaining unread range. If no independent session can be
created, append `blocked` from the real source session with no review ID; do not
fabricate a reviewer. Do not add a token, model-catalog
or native-version gate or invent a PASS.

## 2. Apply the checklist

Evaluate every row, with evidence locators and an explicit outcome:

| Check | Completion criterion |
| --- | --- |
| `source_history` | Read original prompts, calls/results, errors, recoveries and checkpoint across the task interval. Use PASS or BLOCKED only. |
| `task_scope` | Establish the original intent and review boundary; name implicated execution-system owners and maintenance authority. Business-task completion is not this review's acceptance criterion. |
| `failed_tools` | Account for failed calls and recoveries, then assess their causes. Recovery closes the immediate error, not a repeated misuse or guidance defect; retain that finding in the execution rows. |
| `execution_efficiency` | Inspect repeated calls/searches/reads, unchanged retries, progress-only loops, delegation/waiting and escalation. Explain which were justified and identify avoidable waste or missed stall intervention, including after eventual success. |
| `context_alignment` | Compare available/loaded skills, instructions, runbooks and tool docs with the observed sequence. Distinguish missing/stale/ambiguous/conflicting guidance from guidance ignored, tool/harness defects and uncertain causes. Trace installed copies to their canonical owner. |
| `improvement_actions` | Every material pattern has a causal finding, owner, smallest preventive correction and validation/replay plan, or evidence-backed no-change justification. Unresolved actionable findings remain FAIL; missing evidence/authority is BLOCKED. |
| `verification` | Validate the suspected cause or execution-system patch with focused actual checks. Reuse source evidence; run business tests/live probes only when needed for that hypothesis. Distinguish unrun or mocked checks. NOT_APPLICABLE needs a concrete reason. |
| `safe_delivery` | Verify any claimed execution-system correction reached its authorized canonical and installed targets, preserving foreign work and rollback. NOT_APPLICABLE when no correction delivery is claimed; original deployment evidence is context, not a required redeployment. |

For each finding record **native locators → observed consequence → cause and
confidence → owning file/tool → smallest correction → actual or proposed check**.
A failed call alone is not a defect: bounded research, red tests and retries with
new evidence can be justified. Check guidance before changing it; clear guidance
ignored calls for adherence/orchestration correction, not another duplicate rule.
Route a reproducible tool bug to its implementation owner. If causality is
uncertain, say what evidence is missing instead of prescribing speculative edits.
Do not rerun an unchanged command or broad search to manufacture review evidence.

Use PASS, FAIL, BLOCKED or NOT_APPLICABLE for each row. Permit NOT_APPLICABLE only
for `verification` and `safe_delivery`, with a concrete reason. Record material
findings with file/line or native-history locators and the smallest justified
correction. Avoid speculative refactors. A failed checklist row means FIX_NEEDED;
any blocked row makes the overall outcome BLOCKED, retaining known findings.
This checklist and its recorder establish consistency, not cryptographic proof
that the reviewer read history or that native execution succeeded.

## 3. Correct real findings

For a confirmed execution-system defect under executable maintenance authority,
autonomously create a scoped correction worktree in the owning repository and
use an ordinary native coding agent. Provide the original source ID, independent
finding/evidence, canonical target and applicable maintenance authority. Keep the
original business implementation out of this repair unless separately authorized.
Business-task authority alone does not authorize global skills/runtime changes;
record the precise boundary as BLOCKED when maintenance authority is absent.
Resolve managed skills through their source/manager, then verify installed bytes;
patching only an installed/generated copy is not durable delivery.
Append `correction_started` from the correction writer **before the first edit**,
identifying the original directory, separate absolute worktree, branch, writer
and reviewer. Permit at most **two cumulative correction starts per original
source session**, across reviewer changes; keep directory, scope and authority
fixed. The locked recorder assigns the attempt number and rejects further starts.
If the cap is exhausted, append `blocked`; do not change IDs to reset it.
Preserve dirty and unrelated work. Before integration or deployment, re-read the
target HEAD and working-tree state. If the source task has advanced, reassess the
finding and patch against that state; never overwrite newer work or treat an old
checkpoint's acceptance as approval for changed code. Plan-only/report-only
authority permits a finding, not a patch; record the precise execution boundary
as BLOCKED.

Run focused regression checks and repository-required tests. Freeze the candidate
commit and append `correction_completed` with actual command/exit-code/evidence
entries, preserving failed checks. Start a separate fresh-context independent
**patch reviewer** using the same native interfaces. Require its session ID to
differ from both the original
source and correction writer; require original-history access and inspection of
the patch and actual test results. Audit this as `phase: patch`, retaining the
original source ID and adding `writer_session_id`.

Deliver only after independent patch acceptance and required checks pass, using
the task's authorized integration/release target and rollback. Re-run affected
checks after integration; append `delivery` only after verifying the actual
outcome, with the delivered commit and actual checks, never just intent.
If authority or a required capability is unavailable, stop at that boundary and
record BLOCKED. Preserve failed review records; append a new record after repair
or delivery instead of rewriting history. Do not claim the task delivered merely
because a patch review passed.

## 4. Append the audit

Run the bundled stdlib recorder with a JSON object on stdin:

```sh
python3 <skill-directory>/scripts/audit.py --engine openchamber <<'JSON'
{
  "source_session_id": "ses_original",
  "review_session_id": "ses_independent",
  "directory": "/absolute/task/checkout",
  "commit": "0123456789012345678901234567890123456789",
  "scope": ["requested task scope"],
  "task_authority": "original user message locator",
  "checks": {
    "source_history": {"status": "BLOCKED", "evidence": ["session.messages: exact host access error; original history unread"]},
    "task_scope": {"status": "BLOCKED", "evidence": ["Original task authority cannot yet be verified"]},
    "failed_tools": {"status": "BLOCKED", "evidence": ["Original tool results unavailable"]},
    "execution_efficiency": {"status": "BLOCKED", "evidence": ["Attempt/recovery sequence unread"]},
    "context_alignment": {"status": "BLOCKED", "evidence": ["Available and loaded guidance not yet established"]},
    "improvement_actions": {"status": "BLOCKED", "evidence": ["Causes and canonical owners unknown until history is read"]},
    "verification": {"status": "BLOCKED", "evidence": ["Focused causal checks unknown until history is read"]},
    "safe_delivery": {"status": "NOT_APPLICABLE", "evidence": ["Read-only review; no delivery claimed"]}
  },
  "findings": []
}
JSON
```

Replace example values with actual evidence. The recorder derives `status` and
adds schema version 2, timestamp and normalized engine. All eight rows are
required for new reviews. Schema-1 five-row records remain readable, not new
execution-quality evidence. Append reassessments with a locator to the prior
outcome; preserve old bytes and the original identity/authority/attempt budget. Optional `phase` defaults to `task`;
`patch` requires `writer_session_id`. Append each review outcome, including
BLOCKED, to exactly one engine file:
`${XDG_STATE_HOME:-~/.local/state}/agent-fabric/audit/opencode.jsonl` or
`pi.jsonl`. Normalize `openchamber` to `opencode`, `pichamber` to `pi`.
No per-session artifacts or dispatch state are created. Entries carry
`agent_reported: true`; they are reports, not native execution proof. If recording
fails, stop the dependent action, surface the error and retain the result in the
session; never report it logged or proceed with an unrecorded correction start.

### Action events and summary

Omit `event` for backwards-compatible `review_completed` checklist records. For
`review_requested`, `review_started`, `correction_started`, `correction_completed`,
`delivery`, `blocked` or `hook_retirement`, send the common fields
`source_session_id`, `session_id` (the real reporting actor), `directory`, `scope`,
`task_authority`, and nonempty `evidence_refs` (locators, not copied output).
Supply `review_session_id` for all review/correction/delivery actions; require
the reviewer itself to report `review_started`. Use a specific `reason` for
`blocked` or `hook_retirement`. Record hook retirement only after the authorized
removal/disable actions actually occurred; this skill does not install or remove
hooks itself.

For corrections also supply `writer_session_id`, `worktree` and `branch`; for
correction completion/delivery supply `commit` and a nonempty `check` list of
`{"command":"actual command","exit_code":0,"evidence":["result locator"]}`.
Keep command arguments free of credentials. Optional `reason` and `evidence_refs`
describe outcomes without environment dumps, transcripts or raw tool output.
For example, a failed launch can be recorded without inventing a reviewer:

```json
{"event":"blocked","source_session_id":"ses_original","session_id":"ses_original","directory":"/absolute/task/checkout","scope":["requested task"],"task_authority":"user message locator","reason":"session.create unavailable; no review launched","evidence_refs":["source tool error locator"]}
```

Read existing records without creating a log using:
`python3 <skill-directory>/scripts/audit.py --engine openchamber --summary`.
Counts cover recorded requests, starts, completions, checklist passes, blockers,
correction attempts and delivery reports only. `legacy_review_completions` and
`execution_review_passes` separate old delivery reviews from execution reviews. Do not infer uninvoked-task
coverage, native success, or a hung session from these counts alone.

Keep stdin and each encoded row within 64 KiB and the engine log within 16 MiB.
The recorder scans the bounded log under the same file lock before appending;
malformed/truncated history and size limits block writes without repair or
rotation. Require owned non-shared-writable state storage, private (0700)
`agent-fabric/audit` directories, and owned single-link 0600 regular files.
Symlink ancestors and unsafe existing permissions are rejected, not changed.
Preserve blocked logs for operator handling rather than truncating/deleting them.
Obvious credentials are rejected best-effort; sanitize content before logging.
