---
name: harness-review
description: Bounded post-completion execution review. Use observed friction to make one high-impact correction (two maximum), with one visible parent and fresh-context child subagents; not exhaustive auditing or task-delivery QA.
---

# Harness review

Improve **how the agent worked**, not business-task certification. Investigate
observed repetition, stalls, tool misuse or unclear context. Aim for **one
high-impact correction, two maximum—not a quota**. Stop when evidence supports a
cause, canonical owner and safe correction. No worthwhile correction is a valid
outcome; never search every issue to justify the review.

Use native sessions and child subagents with unchanged models, credentials and
permissions. Install no observer, runtime hook, service, dispatcher or schedule.
The static [activation pointer](references/activation.md) is installed explicitly
with `python3 scripts/install_context.py <instruction-file>...`; `--remove` removes
only its marked block. Full instructions load on demand.

## 0. Route the current role before doing anything

Loading this skill does not itself authorize launch. Select the assigned role:

- **Source executor:** the top-level user-facing task owner, including a parent
  orchestrator or direct harness/skill maintenance task, evaluates the completion
  gate below. Run `python3 <skill-directory>/scripts/audit.py --engine <engine>
  --check-source <source-session-id>` before launch; rejection stops launch. Check
  native history for an existing review of this checkpoint; resume it instead of
  duplicating it. Delegation or subject alone does not exempt the owner.
- **Delegated task executor:** return material execution signals and locators to
  the upstream task owner. If unavailable, report deferred ownership.
- **Assigned review supervisor:** this session is the existing supervisor. Record
  `review_started` before history reads. Skip step 1; enter step 2.
- **Investigation or diagnostic worker:** read-only bounded leaf; return evidence,
  coverage and uncertainty. Create no sessions or agents and make no edits/audits.
- **Correction writer:** implement/test only the assigned fix, record correction
  actions, return the frozen candidate. Do not delegate or start another review.
- **Patch reviewer:** inspect the frozen patch and actual checks against the
  bounded brief; return acceptance/findings. Skip steps 1, 2 and 4; no delegation.
- **Explicit review opt-out:** honor the user's instruction and report opted out.

Every dispatch names role, original source ID, existing supervisor ID and entry
point. The supervisor dispatches leaves **as siblings**, never supervisor chains.
Role follows assignment, not native parent/root metadata. Assigned review/correction
episodes never review themselves. Keep the original source ID across every leaf,
correction and reassessment; resolve unclear ownership before dispatch.

### Communicate the closing decision

Every task closure states **launched** (real ID and launch state, not a passed verdict),
**not needed** (evidence-based reason), **deferred** (gate/owner), **blocked** (exact
boundary), **opted out** (explicit user instruction), or **covered by an existing review**
(ID/owner). A child may cite its parent; that parent still evaluates the whole completed task.
Use native history/receipts; no extra report, schema or ledger for this decision.

## When to invoke

Automatic review follows completion of the main task's deliverables, required
checks, authorized integration/deployment, live verification and checkout cleanup.
Blocked, paused, stalled or phase-complete is not finished. Retain friction
locators while working; continue normal diagnosis/recovery. Only an explicit user
request permits early review; label that checkpoint incomplete, not task acceptance.
Expected red tests, productive exploration and a single irrelevant recovered error
need not trigger review. Evaluate recurrence, time and tokens including review
overhead; claim savings only with measurements.

## 1. Source executor: launch one visible parent

Freeze source session ID, engine, absolute directory, commit/history cutoff,
task scope/authority and completion locators (or early-review request). Create
**one fresh visible root review session**, not a fork of the writer. Use native
OpenChamber `session.create` without `returnResult` by default; native API launch
procedures: [OpenCode](references/opencode.md), [PiChamber](references/pichamber.md).
Do not change routing/models or create a private runner.

The brief names the review-supervisor role, step 2, original history locators and
selected friction windows—not an author's diagnosis, final answer or supplied
transcript as evidence. Pass applicable standing maintenance authority with its
instruction locator, canonical roots, integration/install targets and exclusions.
Only investigation is read-only; explicit report-only scope still wins. Authority
must be verified before writes; business-task authority alone grants no global repair.

Immediately record `review_requested` from the source with the real parent ID;
require `review_started` by that parent before history reads. Verify prompt
acceptance and visibility in the intended project. If recording fails, stop the
dependent action. A failed launch records `blocked` from the real source, with no
invented review ID. Check the documented native interface before claiming it absent.

Leave the parent running independently; return its ID and launch state without
polling or copying results back unless requested. Acceptance is not PASS. Reuse
the same parent for the checkpoint. All subsequent delegation uses child subagents.

## 2. Assigned supervisor: bounded child investigation

Keep **one visible parent review session**. Investigation, diagnosis, correction
and patch review use native **child subagents with fresh independent context**,
retrievable child IDs/results and only role-specific locators/constraints. Never
substitute additional root sessions for leaves. If native children are unavailable,
record the boundary instead. Resume an existing child for follow-up; no child delegates.

Dispatch one read-only investigation child. Before dispatch set the budget:
**at most two friction windows and 12 history/tool retrieval calls by default**,
including recovery calls. A window includes original request/authority, relevant
attempt/result/recovery/checkpoint and implicated guidance. Stop early once one
high-impact cause, owner and safe check are established. Select a second issue only
if independent evidence already fits this budget. No whole-transcript sweep.

Use actual original history, not final-only reads or author summaries. Follow
pagination/truncation only for the selected causal window; changed retrieval
strategies count toward the budget. See the host reference for history access.
Return at most two candidates: **native locators → observed consequence → cause
and confidence → canonical owner → smallest correction/check**, plus inspected
and excluded ranges and stopping reason. A failed call alone is not a defect;
new-evidence retries and recovered detours can be justified.

Unrelated hints stay outside scope, not unresolved findings. No worthwhile supported
change within budget permits bounded no-change; uncertainty is not a confirmed
defect. Budget exhaustion alone is not a capability blocker. But missing/truncated
evidence required for a selected claim cannot support PASS: retrieve within budget
or mark that claim unverified/BLOCKED. Never imply full-task coverage.

The supervisor verifies material evidence and any worker blocker with an applicable
safe capability/recovery probe. Final BLOCKED names the exact authority, capability
or hard-budget boundary, alternatives checked, distinct `worker_session_id`, and
nonempty actual supervisor `check` results. Source, parent and child IDs differ.
Cancellation alone does not prove a writer stopped; preserve foreign work and
never interrupt unrelated sessions/shared runtimes for closure.

## 3. Decide within the selected scope

Record four compact checks:

- `evidence_scope`: selected native history/guidance establishes cause, owner and
  authority, or supports a bounded no-change decision.
- `improvement_actions`: selected findings remediated or justified no-change;
  confirmed unresolved findings FAIL; missing required evidence/authority BLOCKED.
- `verification`: focused actual causal/regression checks; distinguish unrun/mocked checks.
- `safe_delivery`: claimed canonical integration/install verification and owned
  checkout cleanup, preserving foreign work and rollback.

Use PASS, FAIL or BLOCKED; only verification/delivery permit NOT_APPLICABLE with a
reason. A blocked check yields BLOCKED; otherwise a failure yields FIX_NEEDED;
otherwise PASS **within stated coverage**, never exhaustive certification. Include
at most two unresolved selected findings in `findings`; place remediated/no-change
outcomes in check evidence. Final report: **problem → correction/no-change → validation**,
at most three bullets plus real blockers. Keep detailed probes/locators in native
history/audit, not repeated transcript dumps.

## 4. Correct selected findings

For selected findings, inspect existing guidance before editing. Clear guidance
ignored calls for adherence, not duplicate rules. Proven tool defects belong in
their implementation owner. Repeated quoting failures warrant an existing or
minimal SSOT script accepting arguments/structured stdin, regression and invocation
pointer. A selected resolved discovery detour warrants a verified reusable entry
point in existing context, not transcript detail. No speculative refactoring.

If causality remains uncertain, allow **one bounded read-only diagnostic child per
source/distinct selected issue**. Record intent before dispatch and child ID/result
afterward in native history; resume it rather than relabeling/replacing it. Its
brief has a concrete causal question, safe probes and explicit stop condition.
No supported fix means no speculative edit: record not-remediated or justified
no-change. Missing required authority/capability is BLOCKED; confirmed unresolved
findings stay FIX_NEEDED. Diagnosis never resets correction budgets.

Verified executable maintenance authority permits autonomous scoped correction
in the canonical owning repository. Use a child correction writer and isolated
worktree; preserve dirty/foreign work and leave business code alone unless authorized.
Before its first edit the writer records `correction_started`: original directory,
separate absolute worktree/branch, writer and parent review IDs, scope/authority.
Allow **at most two cumulative correction starts per original source**, across
resumes/reviewer changes, not two per finding. The first start fixes scope/authority;
later changes require explicit user approval cited by `scope_change_authority`.
Original source/directory and prior history remain fixed. No identity/budget reset.

The writer runs focused regressions and repository-required tests, freezes the
candidate commit and records `correction_completed` with actual command/exit/evidence,
including failures. It returns to this parent. The supervisor starts a separate
fresh-context independent **patch reviewer** leaf through native child delegation.
The reviewer differs from source and writer, reads relevant original evidence,
patch and actual tests, and creates no agents. Record its outcome as `phase: patch`
with original source and `writer_session_id`. Iterate within the cumulative budget;
never manufacture PASS after exhaustion.

Deliver only after independent patch acceptance and required checks. Re-read target
HEAD/dirty state before integration; reassess against newer work rather than overwrite
it or reuse stale acceptance. Verify authorized integration, installed bytes through
the managed source, affected checks and eligible owned worktree/branch cleanup.
Record `delivery` only after actual success with delivered commit/checks. Plan-only
scope, unavailable capability or production-deployment confirmation remains a boundary.
No model/credential/permission changes or unrelated-session interruption. Preserve
negative records; append reassessments instead of rewriting them.

## 5. Append the bounded audit

Run `python3 <skill-directory>/scripts/audit.py --engine <engine>` with a JSON object
on stdin. Completed bounded reports use schema 3, selected by these four `checks`
and required `coverage` text (inspected/excluded ranges and stopping reason):

```json
{"source_session_id":"ses_source","review_session_id":"ses_parent",
 "directory":"/absolute/checkout","commit":"0123456789012345678901234567890123456789",
 "scope":["selected friction"],"task_authority":"native authority locator",
 "coverage":"Inspected request and calls 4–8; remainder excluded; stopped at supported cause.",
 "checks":{
   "evidence_scope":{"status":"PASS","evidence":["native causal-window locators"]},
   "improvement_actions":{"status":"PASS","evidence":["canonical correction locator"]},
   "verification":{"status":"PASS","evidence":["actual focused check locator"]},
   "safe_delivery":{"status":"PASS","evidence":["integration/install/cleanup locator"]}},
 "findings":[]}
```

PASS describes only the declared scope. Historical schema-1/2 records and legacy
eight-row callers remain readable/appendable without migration or rewritten bytes;
new workflow reports use the bounded form. The recorder derives status and adds
schema, timestamp, normalized engine and `agent_reported: true`, not proof of execution.

Action events: `review_requested`, `review_started`, `correction_started`,
`correction_completed`, `delivery`, `blocked`, `hook_retirement`. Supply `event`,
`source_session_id`, real reporting `session_id`, absolute `directory`, `scope`,
`task_authority`, nonempty `evidence_refs`; all review/correction/delivery events
carry `review_session_id`. The parent reports its own start; leaves do not record
new review launches. Corrections add `writer_session_id`, `worktree`, `branch`;
completion/delivery adds full `commit` and nonempty `check` entries shaped as
`{"command":"actual command","exit_code":0,"evidence":["native result locator"]}`.
Blocked actions need a specific reason; post-launch blockers also require distinct
worker and supervisor checks as in step 2. `scope_change_authority` is allowed only
on correction start. Omit `event` for `review_completed`; patch reports require writer ID.

One locked append-only engine file lives under
`${XDG_STATE_HOME:-~/.local/state}/agent-fabric/audit/{opencode,pi}.jsonl`.
OpenChamber normalizes to opencode, PiChamber to pi. Use `--summary` for recorded
counts only, not task coverage, liveness or proof of savings. Keep native locators,
not prompts/transcripts, raw output, environments or credentials. Limits: 64 KiB
stdin/row, 16 MiB log; malformed/truncated history blocks appends, never auto-repair.
Require owned non-shared-writable state storage, private 0700 audit directory and
owned single-link 0600 regular files; reject symlinks/unsafe permissions without
changing them. Secret rejection is best-effort; sanitize inputs. If recording
fails, stop dependent actions and retain results in native history, not claimed logs.
