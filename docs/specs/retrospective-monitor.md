# Retrospective monitor metadata projection

Owner: `scripts/retrospective-monitor.py`. Tests:
`python3 -m unittest discover -s tests -p test_retrospective_monitor.py`.
This is a Linux/Python-standard-library presentation helper and opt-in local
preflight, not a daemon, provider client, repair gate or capture filter.

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
