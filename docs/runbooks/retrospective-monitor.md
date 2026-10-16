# Read-only monitor counters and alert presentation

Contract: [retrospective-monitor](../specs/retrospective-monitor.md).
Helper: `scripts/retrospective-monitor.py` (Python 3 on Linux, no dependencies).
This is a **review-pending presentation rollout**, not live activation of the
Phase1/2 Go core. No daemon or service restart is required for a prompt update.

## Invocation

Use an absolute verified checkout/script and explicit absolute state path:

```sh
timeout -k 10s 120s python3 "$R/scripts/retrospective-monitor.py" \
  --state-dir "$STATE" --previous "$PRIOR"
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

## Replacement prompt block (supervisor applies after independent acceptance)

Replace only the existing ad hoc retrospective counter/alert computation block
for the approved 15-minute OpenChamber task; keep its service checks, identity,
schedule and read-only restrictions unchanged:

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
> the same historical warning as new every 15 minutes. Show RESOLVED as no longer
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
