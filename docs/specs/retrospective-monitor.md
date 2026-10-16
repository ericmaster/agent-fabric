# Retrospective monitor metadata projection

Owner: `scripts/retrospective-monitor.py`. Tests:
`python3 -m unittest discover -s tests -p test_retrospective_monitor.py`.
This is a Linux/Python-standard-library read-only presentation helper, not a
daemon, provider client, repair gate, capture filter or scheduling framework.

## Inputs and safety

Explicit `--state-dir` contains reports/jobs/spool; optional observer-errors may
be absent. No ambient config discovery, environment/credential/service probing,
network, writes, or subprocesses. `--previous` optionally reads a prior helper
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

No existing runtime snapshot/job/report is overwritten by the helper. The scheduler
may redirect stdout to its separately owned snapshot only after independent review
and explicit prompt-update authority. Keep the old snapshot as migration evidence.
Actual schedule/prompt activation and combined Phase1/2 Go wrapper repins are
separate supervisor-owned actions. No restart is needed for a scheduler prompt
change; changing the prompt does not activate the new Go core.
