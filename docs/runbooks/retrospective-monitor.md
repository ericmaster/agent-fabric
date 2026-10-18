# Read-only monitor counters and alert presentation

Contract: [retrospective-monitor](../specs/retrospective-monitor.md).
Helper: `scripts/retrospective-monitor.py` (Python 3 on Linux, no dependencies).
This is a **review-pending presentation rollout**, not live activation of the
Phase1/2 Go core. No daemon or service restart is required for a prompt update.

## Invocation

Use an absolute verified checkout/script and explicit absolute state path:

```sh
timeout -k 10s 120s python3 "$R/scripts/retrospective-monitor.py" \
  --state-dir "$STATE" --previous "$PRIOR" --latest
```

`R` is the reviewed source checkout. `STATE` is the existing retrospective store.
`PRIOR` is the last helper JSON or, on first migration, the old ad hoc snapshot.
All three are supplied by the supervisor, never discovered from ambient history.
Optional repeatable `--known-monitor-session` takes ONLY exact session IDs from
first-party metadata for the named scheduled task. Unknown/new schedule sessions
remain workload until verified. Never classify by title/tool/user prompt, and
never set learning-origin markers to hide capture. Optional `--service-pid` takes
the independently observed numeric PID; a changed PID is unverified identity
change, not proof of deployed version drift. Missing PID means not checked.

The helper writes only stdout. Exit 0 means complete metadata scan; exit 1 emits
a bounded partial/error JSON. Capture stdout even on exit 1; do not replace an
old complete snapshot with empty output on failure. The helper never writes
reports/jobs/spools or its previous snapshot. The scheduler owns any eventual
atomic snapshot replacement. No config/credential/socket/provider is needed.

## Zero-token one-shot gate — hourly; cleanup opt-in not activated

The LLM task `d2785bca-a95c-4252-ab72-7e89a1b5e941` must remain disabled permanently.
The owned timer is hourly, independent of OpenChamber's LLM scheduler. Preserve
`OnActiveSec=1h` and `OnUnitActiveSec=1h`; do not restore five-minute polling.
This source change does not install/restart/activate anything. Example existing
gate invocation (explicit immutable reviewed R required):

```sh
timeout -k 5s 90s python3 "$R/scripts/retrospective-monitor.py" \
  --state-dir /home/ericmaster/.local/state/agent-fabric/openchamber-retrospective \
  --gate-dir /home/ericmaster/.local/state/agent-fabric/monitoring/agf1-openchamber41629/gate-v2
```

For a new fixture the supervisor creates a dedicated directory with mode0700
(no symlinks), then first runs WITHOUT --dispatch: existing work becomes a baseline, subsequent
new or changed jobs/spools become one pending work condition. Pass exact verified
monitor IDs via --known-monitor-session; never infer IDs from names/titles.
Gate writes only gate-v1.json/lock inside this directory; unknown files conflict.
Never put the gate directory in the input store. Reports, snapshots and observer
capture diagnostics alone do not wake workload. Spool/report bodies are not opened
on gate polls; job JSON is bounded and sanitized, with stat-only spool coverage.

Activation requires fresh independent review, first-party CLI authentication and
task-enabled=false verification, and explicit dispatch/timer authority. Only then
append --dispatch (official default target --project-dir /home/ericmaster/nimbler-ops,
--port4098, the task ID above). No custom REST/auth/secret loader is involved.
--openchamber selects the reviewed first-party executable; tests substitute a fake.
Quiet and baseline ticks invoke no scheduler CLI at all. Opt-in cleanup adds
bounded first-party HTTP maintenance even on quiet ticks. Nonquiet dispatch first verifies
the task is paused through official schedule list, then uses official schedule run
--dir /home/ericmaster/nimbler-ops --task d2785bca-a95c-4252-ab72-7e89a1b5e941 --port4098 --json.
The CLI, not this script, owns authentication. Each call has a 30s/1MiB cap;
timeout/invalid output/error after possible session creation permanently trips the
inflight circuit. The gate never retries that claim automatically, even for new work.
The gate requires a new session ID (not pre-run/known IDs), consistent returned task
binding if present, and a subsequent official list receipt for the same exact paused
task/session before acknowledgement. This is not proof of task/prompt completion.
Newly verified monitor IDs reclassify pending provenance BEFORE run: monitor-only
deltas clear without launching; mixed real work remains pending and can dispatch.
Supervisor reconciliation must independently verify that a fresh task receipt belongs
to the persisted claim; never blindly clear it. Then, without --dispatch, use the same
gate/state/CLI target arguments plus `--reconcile-claim <exact-claim-token>` and
`--reconcile-session <verified-new-ID>`. This performs list only, checks paused and
nonrunning state/freshness, acknowledges only unchanged claimed inputs, preserves
later work and clears that exact claim once. A wrong token cannot change the cursor.
There is no automatic not-created reset: if creation cannot be established, preserve
the cursor and escalate for external reconciliation. Do not delete/reset budgets or
change target/model to bypass an inflight claim. Cursor format2 pins target binding;
old opaque-pending cursors fail closed rather than silently losing work.
While first-party task state is running, pending workload coalesces without run calls.

Gate scans use bounded1024-file pages/family and hash-sorted continuation inside the
existing4096-directory-entry ceiling. `decision=scanning` / `coverage_cycle=in_progress`
is partial coverage, not healthy emptiness. Baseline/health resolution waits for a
finished error-free cycle; iterator order cannot starve late entries. A directory
inode/mtime/ctime epoch change restarts that cycle while preserving the last complete
baseline and pending evidence. Prefix content changes are revisited next cycle.
This is eventual sampled coverage, not an atomic snapshot. Stores beyond4096 entries
remain explicitly partial and need supervisor-owned retention/partitioning, not an
increased read cap. Ordinary inventories still use the single-scan limits.

Local visibility is sanitized stdout (intended oneshot journal) and the private
gate-v1.json diagnostic locator. health_changed announces a single health transition,
including recovery; continuing conditions remain visible without repeated alerts.
Health never causes an LLM dispatch. No dashboard/Mattermost integration is claimed;
a future supervisor must choose whether local journal visibility suffices. Service
health is unchecked without an externally verified --service-pid, and PID existence
is not process identity or HTTP readiness. A timer must not keep a stale fixed PID.
An unreadable cursor is preserved and alerts once using its owned lock file.

Scheduled inventory uses --latest, not --full-history: only job-referenced reports
are opened; orphan report names are counted with content coverage unknown. Totals
use stat-only spools with spool_gap_rows=null; spool-only overflow content is unknown.
and alert baselines explicitly change basis. For a separately authorized historical
audit use --full-history; do not raise global read budgets or scan all retained blobs
on periodic gate ticks. No helper/gate change deploys the pending Go decoder.

## Final-output archive and completed monitor deletion — OFF by default

Owning task: `d2785bca-a95c-4252-ab72-7e89a1b5e941` only. Public contract/source
evidence: `../../.artifacts-manager/agf1-monitor-cleanup-official-api-contract.md`.
The installed native API supports DELETE `/api/session/ID`,204; absence of a CLI
delete action is not a blocker. The helper reuses OpenChamber2.3.3's existing
first-party CLI HTTP authentication, never a custom credential getter. User authority
permits global queue data **in memory only** for pending-work exclusion: it is
projected immediately to session ID/count/sending flags, never stored/logged/shown.

After independent review, a supervisor may run this read-only preflight using a
new immutable release containing BOTH Python and sibling Node adapter files:

```sh
timeout -k 5s 60s python3 "$RELEASE/retrospective-monitor.py" \
  --state-dir /home/ericmaster/.local/state/agent-fabric/openchamber-retrospective \
  --gate-dir /home/ericmaster/.local/state/agent-fabric/monitoring/agf1-openchamber41629/gate-v2 \
  --cleanup-plan
```

RELEASE is the supervisor's exact approved immutable directory, not ambient discovery.
Plan mode requires existing private cursor/lock, never creates or changes them,
exports nothing and deletes nothing. It returns at most eight proven IDs/final IDs/
completion/version hashes. Repeating a plan does not persist rotation; counts are
that bounded batch, not a full backlog or zero-pending claim. `known` is classification
only, including manually supplied --known-monitor-session IDs. Only the separate
`cleanup_receipts` records authorize deletion; no legacy known ID is automatically
promoted. Verified fresh schedule run + post-list acknowledgement records future
receipts. Other monitor tasks require their own equally exact authority.

For the legacy session CURRENTLY named by this exact paused task's lastSessionId,
the supervisor can first perform an explicit LIST-only ownership refresh (after
review; not run by implementation):

```sh
timeout -k 5s 40s python3 "$RELEASE/retrospective-monitor.py" \
  --state-dir /home/ericmaster/.local/state/agent-fabric/openchamber-retrospective \
  --gate-dir /home/ericmaster/.local/state/agent-fabric/monitoring/agf1-openchamber41629/gate-v2 \
  --record-cleanup-receipt "$EXACT_FIRST_PARTY_LAST_SESSION_ID"
```

The exact ID must come from the configured task's first-party metadata, not a title.
This reads schedule list once, requires paused/nonrunning/nonqueued task and exact
lastSessionId equality, and saves a body-free task/project/control-origin/native-
origin/source-hash receipt. It does NOT dispatch/archive/delete. A different known
ID is rejected. Past sessions no longer named by that task need retained authoritative
task execution receipts; native metadata.kind=scheduled is not task attribution.
The supplied installed contract does not establish a historical execution-list
endpoint, so report these older IDs as unproven rather than guessing/importing them.

Activation is supervisor-only after fresh GPT-6.1 medium review: preserve paused LLM
cron, native origin41629, service and prompt fields, and append `--cleanup` to the
existing gate invocation. Use a NEW immutable directory with both files; never
overwrite the current release. Keep timer cadence (minimal fragment):

```ini
[Timer]
OnActiveSec=1h
OnUnitActiveSec=1h
```

`--cleanup` is outside the model and adds no model calls. It runs on quiet ticks
before normal work selection; scheduler `cli_calls` excludes maintenance HTTP.
Output: `$R/.artifacts-manager/monitor-output/YYYY-MM-DD/SESSION.md` (private0700/
0600, final visible text only). Never commit actual reports. A done capture job,
native terminal completion, no children/continuations/native inbox and no pending/
sending OpenChamber queue work are all required. Failed/unknown outputs remain.
Native wire shapes are decoded per operation: get/active/inbox are `{data:...}`;
session/message pages remain `{data:[],cursor}`; OpenChamber health/queue remain
flat. Invalid/error/partial envelopes retain history. File and ancestor durability
are separate: every descendant directory entry below the trusted R checkout is
parent-fsynced, including existing entries left by interrupted creation, before
archive publication/deletion. Archive fsync/rename/no-overwrite/readback/hash checks precede the one DELETE and
absence readback. Keep `.attempt.json`/`.deleted.json` and gate known IDs forever
within the existing bounded cache; deletion never causes monitor feedback.

Summary-only outstanding findings are the returned `retainedSessions` IDs/static
codes, eligible/deleted/ambiguous counts and archive locators, not unrelated user
queue text or raw tool/error bodies. `USER_QUEUE_PENDING` means preserve and tell
the user that work is pending; never clear/take the queue to enable cleanup.
Ambiguous attempts reconcile GET-only on later ticks; do not blindly retry DELETE.
If a crash happened before its first DELETE, this conservative state still needs
supervisor reconciliation. Snapshot changes/conflicts keep original output/history.

Latency: next eligible **hourly** maintenance after capture settles, plus bounded
eight-ID rotation and runtime/timer delay, not immediate or strict <=1h. Native
DELETE has no CAS and cascades children; a new child/user continuation arriving
after the fresh checks remains an explicitly disclosed race, never an atomic claim.
No authenticated real preflight/export/delete/UI acceptance was run by implementation.

Checks: `python3 -m unittest discover -s tests -p test_retrospective_monitor.py`
and `node --test tests/test_monitor_cleanup.mjs`, fixtures only. Rollback future
cleanup by removing ONLY `--cleanup` while retaining the new reviewed helper:
its optional `cleanup_after` / `cleanup_receipts` fields are not accepted by the old immutable
gate. Preserve cursor/known IDs/claims/audits/outputs and hourly cadence. Do not
restore an old cursor or reenable the LLM cron to roll back cleanup.

## Replacement prompt block (supervisor applies after independent acceptance)

Replace only the existing ad hoc retrospective counter/alert computation block
for the approved paused OpenChamber task; keep its service checks, identity,
enabled=false, model and read-only restrictions unchanged. Only the hourly
zero-token gate may dispatch it on real new work:

> Run the reviewed `scripts/retrospective-monitor.py` using the explicit approved
> retrospective STATE and prior metadata snapshot. Pass the supervisor's
> first-party verified scheduled-session IDs as repeated exact
> `--known-monitor-session` arguments, and an independently observed PID only if
> available. Use the validated helper output instead of writing metric logic.
> Keep workload and monitor_health counters separate, but surface monitor-health
> capture failures and global observer diagnostics. Interpretations are completed
> semantic deliveries, not skipped rows; use recorded HTTP requests, never rows,
> jobs.attempts or snapshot-to-snapshot sums. Display inventory counts as inventories,
> not monotonic counters. Delivered uncertain/justified choices are genuine semantic
> completions, not actionable repairs or proof of task success.
>
> On first migration use the existing legacy snapshot only as baseline; keep that
> file unchanged. Thereafter compare against the last COMPLETE helper snapshot. Record the
> bounded helper JSON under the monitor-owned `metadata-v1.json` (write a sibling
> temporary file, then atomically replace only that owned file on complete=true).
> Save valid partial output separately as `metadata-v1-partial.json` while retaining
> the last complete baseline, so deferred edges can be compared on recovery. No replacement is
> allowed if the process timed out or did not produce valid helper JSON; report a
> capture failure instead. Partial snapshots are visible but defer new/resolved
> edges. Never claim a healthy empty inventory from malformed or unavailable input.
>
> Notify only NEW warning/health condition fingerprints or meaningful changed
> health conditions. Retain ONGOING conditions in the summary, without repeating
> the same historical warning as new on every invocation. Show RESOLVED as no longer
> observed, not repaired. Historical/outside_interval collection gaps are
> informational; current/unknown scope is unresolved. Unidentified legacy groups
> are not verified unique incidents. Do not turn skipped/unavailable interpretation
> into a model diagnosis. Keep service health separate: a PID change is
> `pid_changed_unverified` until an independent binary/plugin binding check.
>
> Do not change capture filters, task boundaries, source provenance, provider or
> repair guards, monitor schedule, native configuration, wrappers, jobs or services.
> Do not read/print private user/tool/reasoning bodies or raw provider errors. Use
> only the helper's sanitized fields and static error codes/source references.

The exact prior scheduler prompt is not versioned in this repository; this is a
bounded replacement paragraph, not a claim that an unseen prompt was edited.
The supervisor applies it using first-party scheduler tools after review.

## Compatibility and limitations

- Old `interpretations_observed` meant row occurrences; it cannot be relabeled as
  semantic completions. Old monotonic counters/alerts are not imported. Expect a
  one-time counter reset/rebaseline and fewer workload sessions after explicit
  scheduled-session separation. Retained inventories can also shrink with pruning.
- Latest-report alerts and retained-version counters deliberately differ. Scope
  comes from recorded Phase2 metadata, not inferred task boundaries or gap detail.
  Legacy unscoped gaps stay unknown. The helper never weakens collection/repair.
- Identical timestamp/input report attempts cannot prove distinct HTTP sends.
  Missing usage is explicitly unknown. This is not a transport or billing audit.
- File errors expose bounded `source_ref` digests, not potentially secret-bearing
  filenames/content. To identify a local source, use the documented
  `sha256(JSON(["source", "family/filename"], compact separators))` mapping inside
  the approved store; never publish the raw document. Absolute paths outside the
  declared store must not be followed. No helper input grants new filesystem scope.
- Reviewed Go changes still require a separate coordinated wrapper-pin release.
  A prompt-only update does not activate them or resolve legacy missing evidence.
