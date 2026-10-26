---
name: harness-review
description: Review agent execution after completion, roadblocks, repeated tool errors, excessive context searches, misleading instructions or stalled execution; improve harness/tool use, orchestration, skills and context from original history. Visible supervisor-worker review, agent-discretionary or user-invoked; not task-delivery QA.
---

# Harness review

Review **how the agent worked**: avoidable repetition, unnecessary searches,
stalls, tool misuse, orchestration patterns, and skill/context clarity. Task
success is context, not the verdict. A recovered error can still reveal a
preventable execution defect. This is not business-task acceptance or another
full deployment/test run.

Use ordinary native agents and existing host session tools. Keep active models,
credentials and permissions unchanged. This skill installs no hooks and uses no
GLiDE, provider dispatcher, custom HTTP/SDK runner, spool, job, daemon or schedule.
Direct requests to the existing host's native session API are allowed; they use
its existing UI/session runtime rather than creating another runner. The small
[activation pointer](references/activation.md) belongs in native global instructions;
install it explicitly with `python3 scripts/install_context.py <instruction-file>...`.
Only the marked block is managed; `--remove` rolls it back. This is static context,
not event capture or a continuously running supervisor. Full instructions load on demand.

## When to invoke

Invoke at your discretion when a meaningful task reaches its completion checkpoint,
or when a roadblock, repeated errors, or stalled execution warrants an independent
look. Repeated attempts or status-only turns without new evidence are a stall
signal: stop the unproductive loop and invoke before repeating it. A user request
also invokes this skill. Do not require the user to notice trouble or ask first.
A single recovered, irrelevant tool error need not launch another review. Repeated
context searches without new evidence or ambiguous/misleading instructions are
also triggers; expected red tests and productive exploration alone are not.
Optimize recurrence, time and tokens including review overhead; claim savings only
from actual comparisons. Correct proven causes within scoped maintenance authority,
not by adding duplicate rules when existing guidance already answers the question.

Freeze the relevant commit and task-history cutoff. For an unfinished task, identify
that checkpoint as incomplete; inspect the roadblock without certifying completion.
Reuse an existing review for the same checkpoint rather than creating duplicates.
Review/correction episodes never trigger recursive reviews of themselves.

## 1. Start an independent supervisor

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

Pass applicable standing maintenance authority with its native/global instruction
locator, owning roots, integration/install targets and exclusions. Do not turn the
supervisor's entire assignment into findings-only/read-only when that authority
permits correction: only the investigation worker is read-only. An explicit user
report-only restriction still wins; never invent maintenance authority from the
business task. The supervisor must verify authority before its first write.

Verify the session is listed in the intended project and its prompt was accepted,
then leave the review running independently in the UI. Return only its real ID/link
and launch state; do not wait for its verdict, poll to completion, or copy results
back unless the user asks. Creation/acceptance is not a passed review. The reviewer
owns recording its outcome and driving authorized correction/delivery in its own
session. A result-return facility is optional, not a launch prerequisite.

The reviewer is the supervisor; use the worker protocol below for investigation.
Require access to **actual original history** using host
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

## 2. Delegate investigation; verify closure

The visible review root supervises one fresh read-only native worker with a
persisted session/history locator, distinct from source and supervisor. Reuse the
native root-session interface in `references/pichamber.md` (create/prompt/list and
resume the same ID); worker dispatch is not another review and does not consume
a correction start. Do not use ephemeral `--no-session` subagents for this evidence.
Pass only frozen source/history locators, scope/authority, canonical roots, permitted
safe probes and the required report. Keep models/auth unchanged. The worker reads
actual history and guidance, tries safe recoveries and returns native locators,
causes/confidence, coverage and remaining prerequisites; it cannot edit, delegate,
write the final audit or certify task delivery. Keep later correction mechanics
out of its investigation brief. It is not the optional distinct diagnostic child.

Record source identity/cutoff and exact covered/pending ranges in native history
(or existing private out-of-tree state). A requested read is not a completed read:
inspect truncation markers and next cursors; reduce page size, split long entries
by character offset or read retained original output until the pending range is
covered. Preserve content and source locators, not a replacement summary. Context
pressure means checkpoint and resume the same review, not an external blocker.

The worker may report **pending** or a blocker claim, not close the review.
The supervisor independently inspects the claimed boundary and performs an
applicable safe capability/recovery probe before accepting `BLOCKED`. Unread
accessible history, a truncated response or an unfinished attempt means continue
with a changed retrieval strategy, not repeat the same failed call. Inspect native
status/termination after timeout; cancellation alone does not prove termination.
Reuse the same worker continuation; avoid batch-coupling a stalled recovery to
unrelated work. Preserve the existing one-diagnosis/two-correction hard budgets.

Final `BLOCKED` requires an exact external-only action or verified authority,
capability or exhausted hard-budget boundary, alternatives checked, and actual
supervisor probe evidence. Record `worker_session_id`, `reason` naming that boundary
and nonempty `check` entries (command, exit code, native result locators). Source,
worker and supervisor IDs must differ. The recorder rejects new unverified blocked
completions and post-launch blocked actions; pre-launch failures without a reviewer
remain recordable. This validates evidence structure, not whether checks truly ran.
The supervisor owns the checklist, authorized correction and final outcome.

## 3. Apply the checklist

Evaluate every row, with evidence locators and an explicit outcome:

| Check | Completion criterion |
| --- | --- |
| `source_history` | Read original prompts, calls/results, errors, recoveries and checkpoint across the task interval. Use PASS or BLOCKED only. |
| `task_scope` | Establish the original intent and review boundary; name implicated execution-system owners and maintenance authority. Business-task completion is not this review's acceptance criterion. |
| `failed_tools` | Account for failed calls and recoveries, then assess their causes. Recovery closes the immediate error, not a repeated misuse or guidance defect; retain that finding in the execution rows. |
| `execution_efficiency` | Inspect repeated calls/searches/reads, unchanged retries, progress-only loops, delegation/waiting and escalation. Explain which were justified and identify avoidable waste or missed stall intervention, including after eventual success. |
| `context_alignment` | Compare available/loaded skills, instructions, runbooks and tool docs with the observed sequence. Distinguish missing/stale/ambiguous/conflicting guidance from guidance ignored, tool/harness defects and uncertain causes. Trace installed copies to their canonical owner. |
| `improvement_actions` | Apply durable remediation below: record each finding's cause, owner, diagnostic evidence if needed, actual script/guidance correction and validation, or explicit not-remediated/no-change disposition. A plan alone is not remediation. Unresolved actionable findings remain FAIL; missing evidence/authority is BLOCKED. |
| `verification` | Validate the suspected cause or execution-system patch with focused actual checks. Reuse source evidence; run business tests/live probes only when needed for that hypothesis. Distinguish unrun or mocked checks. NOT_APPLICABLE needs a concrete reason. |
| `safe_delivery` | Verify any claimed execution-system correction reached its authorized canonical and installed targets, preserving foreign work and rollback. NOT_APPLICABLE when no correction delivery is claimed; original deployment evidence is context, not a required redeployment. |

For each finding record **native locators → observed consequence → cause and
confidence → owning file/tool → smallest correction → actual or proposed check**.
A failed call alone is not a defect: bounded research, red tests and retries with
new evidence can be justified. Check guidance before changing it; clear guidance
ignored calls for adherence/orchestration correction, not another duplicate rule.
Route a reproducible tool bug to its implementation owner. If causality is
uncertain, use the single diagnostic subagent below instead of speculative edits.
Do not rerun an unchanged command or broad search to manufacture review evidence.

Use PASS, FAIL, BLOCKED or NOT_APPLICABLE for each row. Permit NOT_APPLICABLE only
for `verification` and `safe_delivery`, with a concrete reason. Record material
findings with file/line or native-history locators and the smallest justified
correction. Avoid speculative refactors. A failed checklist row means FIX_NEEDED;
any blocked row makes the overall outcome BLOCKED, retaining known findings.
This checklist and its recorder establish consistency, not cryptographic proof
that the reviewer read history or that native execution succeeded.

## 4. Correct real findings

### Durable remediation

- **Quoting failures → SSOT script.** Reuse an existing helper first; implement
  repeated shell/Python quoting repairs as a reusable script in project `scripts/` or `tools/`
  for project operations, or the corresponding skill's `scripts/` for host/global
  operations. Work in its canonical source. Accept arguments or structured stdin
  instead of rebuilding nested shell code; add a regression test and an invocation
  pointer in the owning context/skill. Follow secret-management/human-pull rules;
  putting code in a script does not authorize secret access.
- **Resolved detours → reusable guidance.** Every resolved broad/repeated search or tool-call detour
  must leave a discoverable path in project context or corresponding skill:
  verified entry point, prerequisites, successful procedure and misleading path
  to avoid. Update the existing owner or link adequate existing guidance; keep
  transcript detail in evidence. Audit-only notes are not durable learning.
- **Unknown cause → one diagnosis.** Dispatch exactly one diagnostic subagent
  per original source and distinct issue, using native delegation and a read-only
  brief with frozen evidence, canonical roots, a concrete causal question,
  permitted safe probes and a bounded stop condition. It must not delegate or edit.
  Record dispatch intent in native review history before dispatch; capture the
  native child ID, outcome and evidence locators, including failed dispatches.
  Check this history on resume/reviewer changes: no replacement diagnosis, no
  renaming the same issue and no recursive review. This does not reset the
  two cumulative correction starts. A supported fix path means proceed with authorized correction,
  validation and delivery below. No supported path, failed dispatch or unavailable
  diagnosis capability means record `not remediated`, evidence and the exact
  limiting reason; never loop on diagnosis or invent a fix. Missing authority or
  capability remains BLOCKED; other unresolved actionable findings stay FIX_NEEDED.

Close each finding as remediated, `not remediated`, or justified no-change in
`findings`/`improvement_actions` evidence. Include diagnostic locators where used,
canonical script/context paths, actual checks and installed-target verification.
Do not label a recommendation or an unvalidated edit remediated.

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
source session**, across reviewer changes. Keep the original directory fixed;
the first correction start freezes correction scope and verified authority for
later starts. Earlier review descriptions may differ, and later user approval may
authorize that first correction; cite its native locator without rewriting history.
The locked recorder assigns the attempt number and rejects further starts.
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

## 5. Append the audit

Run the bundled stdlib recorder with a JSON object on stdin:

```sh
python3 <skill-directory>/scripts/audit.py --engine openchamber <<'JSON'
{
  "source_session_id": "ses_original",
  "review_session_id": "ses_independent",
  "directory": "/absolute/task/checkout",
  "commit": "0123456789012345678901234567890123456789",
  "worker_session_id": "ses_investigation_worker",
  "reason": "Native history access denied; operator must restore the source access",
  "check": [{"command": "native source history read", "exit_code": 1, "evidence": ["supervisor native denial locator; alternate native file also denied"]}],
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
