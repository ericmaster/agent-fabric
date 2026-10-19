# Retrospective monitor metadata projection

Owner: `scripts/retrospective-monitor.py`. Tests:
`python3 -m unittest discover -s tests -p test_retrospective_monitor.py`.
This is a Linux/Python-standard-library presentation helper and opt-in local
preflight, not a daemon, provider client, repair gate or capture filter.

## Completed monitor output cleanup — source-only opt-in

Owner also: `scripts/retrospective-monitor-cleanup.mjs`; tests:
`node --test tests/test_monitor_cleanup.mjs`. No plugin/Go collector changes.
The current owned gate timer is **hourly**, `OnActiveSec=1h` and
`OnUnitActiveSec=1h`, not five minutes. Preserve this cadence and paused native LLM
cron in any future release. No unit/prompt/immutable release is changed here.

`--cleanup` enables maintenance under the existing gate lock, before wakeup
selection, including quiet ticks. Default behavior is unchanged. Cleanup never
dispatches a model; only verified proofs may add known-ID classification. `--cleanup-plan` uses an
existing cursor/lock read-only and never writes cursor/output/audit, exports,
dispatches or deletes. It cannot combine with --dispatch/reconciliation.
Both require the exact task `d2785bca-a95c-4252-ab72-7e89a1b5e941`, project
`/home/ericmaster/nimbler-ops`, UI4098 and managed native41629. Only the private
format2 cursor's separate `cleanup_receipts` or the reviewed composite below qualify; `known` is classification
ONLY and is never migrated into deletion proof. Command-line known IDs alone,
titles or scheduled-kind metadata alone cannot grant deletion authority. The Node
entry independently verifies the canonical gate-v2 private cursor binding. No
other monitor task may receive run receipts. Exclude the explicit supervisor control session.

### Reviewed historical identity amendment

User-authorized legacy cleanup additionally permits exactly these task ID/name pairs:
`d2785bca-a95c-4252-ab72-7e89a1b5e941` / `AGF-1 OpenChamber session monitor`,
`55d5f2b9-3ea8-4223-91fe-7cffdc71bcef` / `Monitor operativo de agentes — progreso y loops`,
`ce0092b5-5552-4d14-a695-592ff7b48d96` / `Monitor operativo de agentes — persistente`.
The installed first-party schedule.list scoped to the exact project supplies current
stored prompt definitions in memory only. Require unique IDs/names, disabled tasks,
nonrunning state, no targetSessionId, exact optional directory and prompt execution
without goal/agent attachments. No candidate-derived hash is a recognized template;
only exact current stored prompt SHA256 is recognized. Historical/expanded templates
are NOT recognized by this release. Conflicting identical prompt definitions retain.

Complete project native inventory (8 pages x100, unique IDs and cursor safeguards)
discovers scheduled-kind exact-title candidates beyond the classification cache.
Every discovery page explicitly supplies the `directory` query parameter; the
directory header alone does not scope the installed session-list API.
Titles must equal exact task name + creation minute formatted in the task timezone
(host default if unspecified), using the installed yyyy-MM-dd HH:mm contract.
Explicit task lastSessionIds are also inspected, never automatically authorized.
An incomplete inventory aborts the maintenance batch, not a silent prefix success.
Union with cached IDs is capped1024; durable cleanup_after rotates eight IDs even
past failed candidates. It is independent of classification membership. Only fully
verified identities returned by maintenance join known; manual known IDs alone
never acquire deletion authority. Discovery/retention never wakes an LLM.

### Classification must survive deletion and adapter crashes

Mutation uses two bounded adapter phases under the existing Python gate lock.
First perform read-only classification with the unchanged full receipt/composite
and independent safety gates. Python validates the reduced result, merges only
verified IDs into known, filters only their pending/baseline provenance and durably
saves the existing private format2 cursor (file fsync/replace/directory fsync).
Only after this barrier may it start the destructive adapter phase. Both phases
share the existing50s parent budget; no adapter writes the Python-owned cursor.
Plan-only remains one read-only phase with no cursor writes.

The adapter never archives/deletes a newly discovered ID absent from its authorized
input known snapshot: it can return verified classification for a later phase/tick,
but manual known membership still cannot substitute for receipt/composite proof.
The production entry verifies known against the canonical owned0700/0600 nofollow,
bounded schema/revision/binding cursor before any maintenance. Existing archive
durability/readback and fresh pre-delete safety checks are unchanged.

On ANY adapter/parse/timeout/barrier failure the current tick is incomplete with
CLEANUP_UNAVAILABLE health and zero dispatch; preserve real pending provenance.
Repeated failures announce one stable health edge, recovery one edge. Next tick
reads the already durable classification BEFORE workload selection, even when
native history disappeared after DELETE204 and before adapter stdout. Existing
attempt reconciliation remains GET-only and returns already-known classification;
it never imports an arbitrary archive filename/receipt as classification. A corrupt
cursor fails closed before any adapter/workload/dispatch and is not overwritten.
No new journal, HTTP route or cross-language cursor writer is introduced.

`agf.monitor-legacy.v1` is a distinct ephemeral composite attestation, NOT a native
historical task execution receipt: bind exact task ID/name/project/control4098/
producer41629/session/kind/title, initial native authored USER ID and instruction
SHA256, recognized current stored prompt SHA256 and source `schedule.list.current`.
It is recorded in the final archive/audit without prompt text; not cached as a
run receipt. Fresh pre-delete snapshot recomputes it from native user.text and
the same recognized task definitions, and full metadata/history version must match.
Exactly one authored user message is mandatory even if later instructions repeat
the prompt. Native synthetic context is nonhuman: allow only bounded native
synthetic records before that instruction with delivered knowledge metadata, never
assistant content as instruction proof. Unknown roles/history retain.

Scheduled metadata is a closed plain object: only openchamber with required kind
scheduled and optional knowledge_context_delivered/assist. Knowledge permits the
installed sorted, unique guide-only signatures (empty or l:on, m:on:c/p, r:on,
w:ask/auto; mutually exclusive alternatives); pinned note/plan/memory revisions
remain unreviewed and retained. Assist has exactly recap<=320, suggestion<=500,
forMessageID and safe integer generatedAt; at least one nonempty string, final ID
match and completed<=generatedAt<=now, no more than24h after final completion.
Assist is an idle proposed suggestion, NOT a submitted human message. Unknown
linked/pin/goal/review fields retain; full metadata remains in snapshot comparison.
Only visible final text is exported; assist strings never enter archives/logs.
Official cursor previous/next values are absent, null or nonempty opaque strings
<=4096; unknown keys/types retain. Non-null child cursors still retain the parent.

Each deletion receipt is schema1, expected producer_origin41629, and a body-free
source receipt binding task_id/session_id/directory/control origin4098/enabled=false/
last_status, with kind `schedule.run+list` plus matching run_session_id, or kind
`schedule.list.backlog` with null run_session_id. source_sha256 is SHA256 of canonical
JSON `["cleanup_receipt", source]`. This is integrity checking inside the private
owned cursor, not cryptographic authentication against its UNIX owner. Only the
verified fresh run/post-list flow creates run receipts. `--record-cleanup-receipt ID`
is a separate, explicit first-party LIST-only ownership refresh: require the exact
paused task's current lastSessionId=ID and nonrunning/nonqueued state, then durably
save the reduced source receipt. It cannot dispatch/export/delete and cannot combine
with other mutation/plan modes. This enables verified current legacy backlog without
promoting manual known IDs. Historical IDs no longer named by that task need either
retained authoritative execution receipts or the distinct reviewed composite above,
never kind/title alone. No historical execution-list endpoint is established.

Wire decoding is operation-specific: native session.get/active/inbox200 responses
are EXACT `{data: record/map/array}` envelopes (native2.0.30 session protocol289/
276/609); unwrap only these routes after shape validation. Active map values must
be `{type:"running"}`. Native session.list and message.list remain `{data:[],cursor}`
(session184-190; message71-77); reject errors/partial/unknown envelope fields and
malformed cursor. OpenChamber health and global queue stay their own flat schemas.
DELETE204 remains empty, not an envelope/boolean. Tests traverse the public HTTP
transport with recorded-shape fixtures, not only already-decoded internal mocks.

Reuse installed OpenChamber2.3.3 `bin/lib/cli-http.js` requestJson for first-party
authentication/proxy behavior; no new credential getter, route, dependency, backend
start or runtime-store mutation. Fixed loopback4098, no redirect,4s/request,
1MiB/response,30s maintenance budget,45s adapter/50s parent timeout. The separate
stdin payload cap is1MiB: it must cover all1024 supported receipts sent before
eight-ID batch selection. IDs are `ses_` +1..64 ASCII alphanumerics (max68 bytes);
known/receipt counts each max1024. Receipt task/project/directory/origin/kind fields
are the exact pinned strings above, not arbitrary paths; source hashes are64 hex
characters, schema=1, enabled=false, and status is the closed enum (max7 characters).
The largest Python-default-JSON supported payload (1024 max-length run receipts,
max-length known IDs/after, longest status, plan=false) is697465 bytes, below1048576.
Only stdin is adjusted; response, cursor-file, output, scan and eight-ID batch caps
are unchanged. Read and byte-check each stdin chunk before parsing; reject oversized
or malformed input. The transport regression pipes real Python serialization through
the production Node reader/authority/maintenance with fixture-only jobs, proving
successive eight-ID ticks advance at the full cache ceiling without HTTP or exports.
First-party
authentication may answer an initial401 with its normal login/challenge replay;
no transport-error/DELETE retries are added. DELETE `/api/session/ID` succeeds
with204/no body, never boolean. A fresh health receipt must identify ready41629.

Require a matching canonical capture jobs/ID.json (owned0600, nofollow), status=done,
producer41629, freshly checked before deletion. Reject unowned/foreign roots,
non-scheduled or unknown/linked metadata, forks/parents/subpaths/reverts/archived,
children, active execution, native inbox, manual continuations/multiple user messages,
failed/interrupted sessions, unsupported history, incomplete or absent visible final.
Read whole native message pagination (max8 pages of100), detect repeated cursor/
truncation; order is supplied on the FIRST page only (native cursors carry it).
Require terminal succeeded idle after a completed assistant finish=stop.

Explicit user authority permits GET `/api/message-queue` solely for pending-work
exclusion. Immediately project its complete bounded response to sessionID/count/
sending flags and discard all prompt/content/attachment/context bodies. No global
queue text is saved, logged, hashed, shown, or included in diagnostics. Unknown,
duplicate, malformed, partial or oversized queue snapshots fail closed. Any pending
or sending work retains that session, even when native idle/inbox/history look done.
Check this queue before archive and freshly immediately before DELETE. It is a
separate queue: native idle alone is insufficient and session.deleted clears it.

Only final visible assistant TEXT goes to the output file, never whole history,
user/tool/reasoning/synthetic/hidden bodies. Unknown text visibility flags retain
the session. Metadata records task/session/project/producer/final ID, completion,
source generation and hashes. Full snapshot hashes detect changed history/identity/
completion on fresh read; bodies remain ephemeral. Possible secret-header/key
patterns in final text retain the session rather than exporting them.

Output root: R `.artifacts-manager/monitor-output/YYYY-MM-DD/SESSION.md`, owned
directories0700/files0600, untracked. Refuse symlink ancestors, unsafe ownership/
modes/nonregular/multiple-link files, existing conflicts and all write/fsync errors.
For creation, validate ancestors from the declared durable checkout anchor R, and
fsync EACH parent entry while walking every descendant (including existing entries
left by a prior interrupted mkdir), before descending/archiving. This includes R
when .artifacts-manager is created and the artifact root when date directories are
created. Any mkdir/parent-fsync failure prevents deletion. Publish private temp -> file fsync -> rename within unique private staging -> atomic
no-overwrite hardlink to final -> unlink staging -> directory fsync -> reopen and
verify exact bytes/hash. The hardlink prevents rename-overwrite of an existing file.
Matching immutable output may be reused; unrelated output is never overwritten.
Persist an immutable `.attempt.json` audit BEFORE a fresh second snapshot and the
single DELETE; reverify the archive before deletion.204 requires exact GET404
absence readback before `.deleted.json` tombstone. Timeout/crash/error is ambiguous:
later ticks perform GET-only reconciliation using verified archive+attempt proof,
never blindly repeat DELETE. A crash before DELETE can therefore conservatively
require supervisor recovery. Keep archives/audits and the gate known-ID cache.

Process at most8 sorted IDs/tick with durable cleanup continuation; advance past
blocked candidates, preserve bounded progress on budget exhaustion. Plan-only
does not advance the cursor. Cleanup warnings/retained IDs are safe metadata,
never workload or LLM-wakeup input; scheduler cli_calls excludes maintenance HTTP.
Legacy backlog without exact receipts or verified composite remains retained, not silently reclassified.
Native API has no CAS and deletion cascades children: fresh checks cannot eliminate
a user continuation/new child arriving in the final check/delete window. Existing
children always block. This residual race is disclosed, not an atomic guarantee.
Latency is next eligible **hourly** tick after capture settles, plus bounded rotation
and runtime delay, not an immediate completion callback or strict one-hour promise.
Actual backlog counts, authenticated transport, export/delete and UI acceptance are
deferred to supervisor activation after independent review. Roll back only source
or future cleanup opt-in, never live state/output/ownership evidence/user work.

## Inputs and safety

Explicit `--state-dir` contains reports/jobs/spool; optional observer-errors may
be absent. No ambient config discovery, environment/credential/service probing,
network, writes, or subprocesses in inventory mode. `--previous` optionally reads a prior helper
snapshot; `--service-pid` is only an externally observed integer, not verification.
Repeated `--known-monitor-session` supplies exact first-party verified native
session IDs. No title/tool/prompt-based classification or learning-origin spoofing.
Unknown sessions remain workload; known sessions' capture gaps/job failures remain
visible separately as monitor_health, and unattributed observer errors are global.

Open absolute paths component-by-component with directory descriptors and
O_NOFOLLOW; regular files only, no symlink traversal (including directory parents),
nonblocking opens. Fixed limits: 8 MiB/file, 64 MiB read bytes, 1024 JSON files
per family, 4096 directory entries per family, 4096 active alerts, 64 displayed
errors. Missing required directories,
bad/duplicate-key/nonfinite JSON, unsupported schemas, malformed metadata,
oversized files, symlinks and limit exhaustion are visible static error codes,
bounded source-reference hashes and `complete:false`, never silent zero success.
Partial scans defer new/resolved comparisons, not falsely resolve absent evidence.

Input JSON may contain bodies; only declared metadata is projected. No detail,
title, user/tool/reasoning body, provider error, free-form reason, execution path,
filename or input literal is emitted or hashed for identity. Session/record/task
IDs are represented only by domain-separated metadata digests. Output is
deterministic sorted JSON to stdout, exit 0 for a complete scan, 1 for partial;
argument failures emit a fixed safe JSON error, never echoed input or traceback.
Owned integer metadata is non-boolean and bounded to signed 64-bit maximum;
counts are nonnegative and report interval start permits -1. Probability and
confidence must be non-boolean numbers in [0,1], checked without converting an
arbitrary JSON integer to float. Unrepresentable UTC timestamp normalization is
invalid metadata, just like malformed dates. One invalid document leaves other
valid inventories visible in a partial result, with no new/resolved edge claims.

## Counts, not invented calls or verdicts

Schema `agf.retrospective-monitor.v1` provides workload and monitor_health buckets.
Counters are **current retained inventories**, not cumulative/monotonic across
snapshots. Sessions are distinct observed IDs across reports/jobs/spool. Report
versions count files; copied reports do not create new report attempts. Reports
are schema v2; unsupported historical schemas are visible errors, not translated.

Raw signal rows and interpretation rows count retained report occurrences.
Unique signals use root/session/task epoch/rule/stable signal ID. Epoch is boundary
source + exclusive start seq (and host task identity when present); end seq and
native synthetic per-final task-ID suffix do not create epochs. Same-session
explicit new intervals stay distinct even when signal IDs repeat. Only the six
documented detector rule enums are emitted; unknown rules are invalid metadata.

Semantic completions require status=interpreted, delivered=true, a valid
actionable/justified/uncertain choice and finite in-range probability/confidence.
Count unique attempt+signal deliveries, not skipped rows. Valid provider-delivered
uncertain/justified choices ARE completed semantics but not actionable repairs.
Skipped/unavailable/invalid-response uncertain rows remain separately labeled;
neither skipped nor aggregate interpretation status proves a model call.

Requests/retries come ONLY from `usage.requests/retries`, once per distinct report
attempt metadata: root/session/epoch/generated_at/input_sha256. Never sum mirror
interpretation.requests, jobs.attempts or prior snapshot counters. Identical copies
are one attempt; conflicting usage for one identity is an error. Missing usage is
unknown, not inferred zero: expose known sums and unknown-attempt counts, with
total=null if unknown or the scan is partial. Identical attempts within the same timestamp/input cannot
be distinguished without a future transport attempt ID; these are recorded
metadata counts, not an independent transport audit or billing total.

## Gap conditions and alert edges

All retained gap row occurrences are counted separately from unique identified
gaps. Phase2 stable gap IDs, else structured record/range metadata, identify a
gap; dedup namespaces include root/owner/kind. Recorded scope is
historical/current/outside_interval/unknown; missing scope is UNKNOWN, never
inferred historical from text. New Phase2 overflow remains unknown. No completeness
or repair authority is recomputed by this helper.

Legacy gaps without anchors cannot support an honest unique-incident count.
Expose unidentified groups plus peak row multiplicity, explicitly not distinct
issues. Group by session/epoch/kind; never parse/hash diagnostic detail to fabricate
record identity. Increased multiplicity changes that group's alert condition;
equal repeated historical report versions do not. Distinct anchored new gaps
always have distinct fingerprints. Counters retain raw repetition diagnostics.

Active alerts use the latest generated report PER SESSION (ties prefer later
resolved start/end seq, then deterministic metadata/source hashes), not every old
report version. Earlier reports remain in inventory counts.
Resolved report start/end are independent of every gap's optional local bounds;
record-only, ranged, reordered or duplicate gaps never change report ranking.
Known historical/outside gaps are informational; current/unknown are warnings.
Fingerprints include root/session/gap identity/scope and epoch for nonhistorical
conditions. Historical anchored gaps ignore evolving epochs to avoid re-alerting
unchanged history. Job failed conditions and observer diagnostic file identities
remain health alerts, never hidden by monitor-session exclusion. Latest job
attempts are gauges only, not HTTP request counts.

Compare active fingerprint sets to a valid prior helper snapshot: new, ongoing,
resolved. A resolved condition can reappear as new after a complete absent
observation. Resolved means no longer observed, NOT repaired or task-success proof.
If inputs are partial, defer edges. An old ad hoc snapshot is a migration baseline:
old counters/alerts are not imported; first helper inventory establishes a baseline
without paging all existing conditions as new. No schema/counter continuity is
claimed across that migration. Unsupported/corrupt prior snapshots are errors.
Legacy snapshots with `counters_monotonic` may have an integer schema or no schema;
both are baseline-only, never imported as semantic/request or alert authority.
The scheduler retains its last complete helper baseline on partial runs and saves
partial output separately; otherwise edges deferred during missing evidence could
never be compared to the last trustworthy observation.

Service PID changes are `pid_changed_unverified`, never confirmed binary/version
drift. The helper cannot verify processes, service health, executable digests or
deployment. Existing separate service checks remain the scheduler's responsibility.

## Rollout boundary

### Zero-token gate (inactive until separately authorized)

Repair contract: cursor format2 stores session-keyed input versions in both baseline
and pending provenance. Old opaque pending hashes cannot migrate automatically.
Exact first-party receipt IDs reclassify pending entries before any run; remove only
proven monitor entries, never mixed real work. A run acknowledgement requires a
fresh session distinct from pre-run lastSessionId and all known monitor receipts,
consistent optional returned task/directory binding, and a subsequent official list
receipt binding the exact paused task to that same ID. This proves a scheduler
receipt, not delivery, task success or prompt completion. Persist claim before calls;
contradictory/nonzero/malformed responses retain the claim and pending provenance.

Gate scans sort at most4096 directory entries per family, process at most1024 JSON
files per family per tick and persist metadata-hash next-key/coverage-cycle state. Complete
baseline reconciliation occurs only after every family finishes the cycle and its
directory inode/mtime/ctime epoch is rechecked. Directory mutation restarts the cycle,
retaining pending work and the last complete baseline. Content mutation in an already
sampled prefix is detected in the next cycle; coverage is sampled, not an atomic
filesystem snapshot. Partial cycles are explicitly incomplete and never dispatch or
resolve health. Stable stores of1025..4096 entries make bounded eventual progress,
independent of iterator ordering. Beyond the existing4096-entry safety ceiling the
gate remains visibly partial; supervised retention/partitioning is required, never
silent prefix success. Per-tick JSON read bytes retain the64MiB cap. Cursor state
remains bounded by that directory ceiling and the existing8MiB cursor-file cap.

Health is computed AFTER dispatch/reconciliation transitions. An inflight claim
always exposes structured DISPATCH_AMBIGUOUS / RECONCILIATION_REQUIRED health, with
one edge at failure, ongoing identical state on later polls, and one recovery edge
on explicit authoritative reconciliation. No new work or binding change resets it.
Manual `--reconcile-claim SHA --reconcile-session ID` (without --dispatch) requires
the exact persisted claim token and a paused, nonrunning first-party task receipt
for a fresh ID. The supervisor must independently establish that receipt belongs to
the claim. It acknowledges only unchanged claimed provenance, preserving subsequent
work, caches the exact ID and clears that one claim once. It does not dispatch.
No not-created assertion, blind reset, automatic retry or unverified deletion is
provided. If creation cannot be established, retain the claim for external recovery.

`--gate-dir ABSOLUTE` selects a local one-shot gate, not the inventory CLI.
The directory must already exist, belong to the effective user, have mode 0700,
and contain only this gate's `gate-v1.json` and `gate-v1.lock` files. Files are
owner-only, nonsymlink regular files; cursor replacement is atomic. A nonblocking
fcntl lock serializes scan, claim and dispatch. First complete observation baselines
old work without dispatch. Failed scans never advance the last complete baseline.
Only prospective new/changed jobs and spool stat identities wake workload, including
continued activity in existing sessions. Report-only changes and observer errors
never wake workload. Exact known monitor IDs exclude their jobs/spools from work;
their failed jobs and observer errors remain local health conditions. No title
classification or capture exemption exists. Spool contents are never opened by the
gate; its coverage is explicitly stat-only, not JSON/content validation.

Bounded metadata scans use existing per-tick entry/file/byte caps with the gate's
persisted coverage continuation above (inventory CLI remains a bounded single scan).
Jobs are parsed through
the safe reader; spool/report directory entries are stat-only, with owner/regular
file/symlink checks. Unknown filenames, oversized inputs, missing directories and
entry caps produce partial scans, never healthy empty inventories. Health alerts
emit only on fingerprint transitions, including recovery. An optional externally
verified `--service-pid` is checked for local process existence, not binary identity.
No service argument means service health is unchecked. Health never launches a model.

Without `--dispatch`, real work remains pending with zero CLI calls. With explicit
activation, the gate uses ONLY `openchamber schedule list/run --dir ... --port ...
--json`; it requires the exact task disabled, and defers coalesced pending provenance
while task state is running/queued. Exact lastSessionId from this first-party
task receipt and independently bound fresh run sessionId are cached before releasing
the lock. Only a fresh post-run task receipt acknowledges pending work.
Before a run the gate persists an inflight claim. Any failure, malformed response,
timeout or interrupted run leaves a durable ambiguous claim: no automatic retry,
even for subsequent work, until supervisor reconciliation. This conservative circuit
breaker intentionally favors no duplicate tokens over availability. List failures
trip the same circuit; no credentials are discovered or printed. CLI output is
bounded, memory-only and discarded; errors expose static codes only. The authoritative
full diagnostic is the private cursor, never raw credential-bearing CLI stderr.

`--latest` inventory reads jobs first and only their authoritative report_path
within the store's reports directory (owner/regular/nofollow enforced). Unreferenced
report names are counted as `unreferenced_reports`, content coverage unknown, not
silently included. Totals describe latest job-referenced reports, NOT all history or
all HTTP calls. Explicit `--full-history` preserves historical inventory behavior;
the legacy no-flag CLI remains compatible. Scheduled invocation must use --latest.
Latest mode also uses stat-only spool inventory, never parses retained spool bodies;
spool_gap_rows is null (unknown), not invented zero. Spool-only overflow content
requires a separately authorized full audit or first-party collector diagnostics.
Gate output never runs inventory or reads retained report/provider bodies on quiet
polls. Snapshot/cursor metadata is outside the input store, preventing feedback.

All gate changes are source-only. No timer, task enable/disable, actual dispatch,
service restart, production capture change or credential bridge is authorized by
implementation. Rollback removes only reviewed source changes and, after activation,
the separately owned gate files; never runtime jobs/spools/reports.
An unreadable/corrupt cursor is preserved, never reset. Its single static health
fingerprint is recorded in the already-owned lock file to suppress repeated alerts.
Inventory basis changes rebaseline alert comparisons; they never falsely resolve
unreferenced history. Unknown orphan report contents remain an explicit coverage
limitation. Stat-only spool coverage cannot diagnose syntactically corrupt content;
the collector's job/observer failures remain the metadata health authority.

No existing runtime snapshot/job/report is overwritten by the helper. The scheduler
may redirect stdout to its separately owned snapshot only after independent review
and explicit prompt-update authority. Keep the old snapshot as migration evidence.
Actual schedule/prompt activation and combined Phase1/2 Go wrapper repins are
separate supervisor-owned actions. No restart is needed for a scheduler prompt
change; changing the prompt does not activate the new Go core.
