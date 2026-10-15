# AGF-1 — signal-first task retrospective

## Objective / ROI

Find observable task friction cheaply and reproducibly, then use GLiDE only to
interpret candidate evidence in context. Retain useful diagnostics even when a
task fails or the provider is unavailable. Repair only independently verified,
safe instruction defects; a detector hit never authorizes a write.

## Authority, scope and current delivery

- Execution root R: `/home/ericmaster/tools/agent-fabric-worktrees/agf1`, branch
  `agf/task-retrospective`; docs revision starts at
  `9a26ba78091240d94569043f21ddec0091a56624` (newer than expected `3508905`).
  Tracked tree initially clean; all existing untracked QA evidence is preserved.
  `/home/ericmaster/tools/agent-fabric` contains user work: never modify it.
- This atomic delivery changes documentation only. No code, provider requests,
  credentials, browser work, installation, activation, commit, push or deployment.
  GPT-6.1 Sol medium remains the user-selected implementation model.
- Inputs within R: `AGENTS.md`, `docs/specs/agent-fabric.md`,
  `docs/specs/task-retrospective.md`, `docs/runbooks/task-retrospective.md`,
  `.artifacts-manager/agf1-final-qa-3508905.md`; relevant implementation/schema
  inspection is read-only. This plan is the canonical phase/acceptance authority;
  the backing spec owns exact rules, thresholds, report and safety semantics.
- Missing/unreadable/ambiguous required locators block dependent work with the
  exact gap; never search other roots to repair a locator.
- Docs rollback: restore only this delivery's tracked documentation diff to the
  verified starting bytes, preserving concurrent edits and all QA artifacts.
  No staging/commit until independent reviewer passes and supervisor authorizes.

## Replacement decision (not a second engine)

The former model success gate, full-trace pagination with classification of every
call, requirement-omission analysis, and `N>=20 / avoidable rate>5%` routing are
**superseded**, not optional alternate paths. Existing code at this baseline
still implements that design; these docs specify its replacement, not completed
implementation. Prior non-trigger QA remains legitimate historical evidence,
not a reason to force a different live verdict or rerun until one triggers.

Pipeline: normalize/redact → deterministic signals/report → no candidates: stop;
candidates: bounded GLiDE interpretation → actionable / justified / uncertain
→ guarded advisory or independently validated repair. Task terminal outcome is
separate observed data, never a modeled success gate or retrospective acceptance.
Failed/aborted/incomplete tasks may yield signals/advisories; writes still require
complete causally relevant evidence, safe redaction, validation capability and
all existing ownership/provenance/native-role/review/QA/WAL/conflict guards.

No new dependency, framework, generic rule engine, daemon, storage service or UI.
Use existing Go CLI, local files, OpenCode observer and native visible roots.
No semantic script-similarity engine, shell-command NLP, fabricated duration/token
metrics, requirement-omission feature, cross-harness expansion or executable fixes.
Keep fixed documented configurable thresholds and stable versioned rules.

## Phase 1 — deterministic detector/report and frozen fixtures

**ROI:** useful, inspectable friction evidence without model cost or availability.

**Actionables:**
- Implement spec-defined terminal failure, exact repetition with observed equal
  prerequisite fingerprints, exact ephemeral-helper creation and search/context
  sequence rules. Unsupported schemas abstain with reasons; no inferred hashes,
  unchanged environment or semantic intent.
- Persist exact call IDs, rule/version IDs, thresholds, observed outcome,
  coverage/abstentions and omitted counts independently of GLiDE. Redact locally;
  retain partial safe reports without promoting them to repair eligibility.
- Freeze synthetic positive/negative fixtures before evaluation, including red
  tests, polling, legitimate research, changed/missing fingerprints, unknown tool
  shapes, duplicate/out-of-order IDs, secret canaries and evidence caps.

**DoD:** targeted detector/report/CLI tests prove exact counts, threshold edges,
stable evidence, no network required, and no avoidability/write claim from hits.
`go test ./internal/retrospective/... ./cmd/agent-fabric/...` passes; synthetic
public replay writes the expected report on failed/aborted input and unavailable
provider. Publish candidate digest and evidence → fresh independent reviewer;
supervisor closes the phase before Phase 2. Review scope is not live calibration.

**Rollback:** remove only this phase's detector/report changes or revert to the
frozen pre-phase revision without deleting reports/user work; global opt-out
continues to prevent native capture/dispatch.

## Phase 2 — compact interpretation and single pipeline integration

**Depends on:** reviewed Phase 1.
**ROI:** spend model effort on meaning, context acquisition and tool bottlenecks,
not mechanically evident events or every successful call.

**Actionables:**
- Replace (do not retain alongside) the success/per-call/omission evaluator with
  bounded candidate interpretation using intent and relevant ordered evidence.
  No candidates means zero model requests. Preserve signal reports on provider
  failure, uncertain answers, truncation or unsuccessful source outcome.
- Validate cited supplied IDs and evidence completeness; conservative semantic
  actionability uses probability>=.90 and confidence>=.80, not detector confidence
  or a calibrated likelihood. Justified/uncertain signals remain visible.
- Integrate public analyze/ingest/drain/dispatch paths and versioned report/config
  migration. Legacy reports stay inspectable but cannot acquire new write
  authority; remove obsolete routing settings with explicit migration errors.
- Keep collection, redaction, provenance, trusted origin, learning suppression,
  native role/model, independent reviewer+QA, source hash, WAL, retry, dedup,
  protected path, rollback and concurrent-user-edit safeguards intact. Guard
  unsuccessful-source repair explicitly; never label its original task successful.
- Update affected specs/tests/runbook/README with actual implemented behavior.

**DoD:** public-path mock tests prove no-candidate/no-network, failed-source
advisory, unavailable/uncertain provider, justified controls, cited actionable
issue routing, evidence-cap no-write, legacy rejection/migration, and guarded
dispatch with no second evaluator path. Run Phase 1 targeted commands, plus
`go test ./...`, `go build ./cmd/agent-fabric`,
`python3 -m unittest discover -s tests -p 'test_supervisor_ledger.py'`,
`python3 -m unittest discover -s tests -p 'test_retrospective_release.py'`,
`npm ci --prefix hooks/retrospective`, `npm test --prefix hooks/retrospective`,
`npm run typecheck --prefix hooks/retrospective`.
Publish frozen candidate/evidence → fresh independent reviewer before closure.

**Rollback:** global disable first; restore only phase-owned implementation and
compatible config, preserve old/new reports and journals; never overwrite a
user edit or replay an ambiguous repair/session submission.

## Phase 3 — controlled detection and repair acceptance

**Depends on:** reviewed Phase 2.
**ROI:** reproducible safety/function proof without chasing stochastic live hits.

**Actionables / DoD:**
- Detection acceptance runs the frozen labelled positive AND negative fixtures
  through public producer/runtime paths. Compare exact rule/call IDs, counts,
  abstentions, coverage and routing. No repeated live resampling to get a hit.
- Repair acceptance uses an explicitly labelled deterministic interpreted-issue
  fixture injected **only in the test harness**. No production evaluator bypass,
  environment toggle or fake provider verdict. Prove original fixture task fails
  → actual independent reviewer approves the exact proposed diff/digest → guarded
  repair → same task succeeds; negative regressions, dedup/origin suppression,
  conflict, interruption/WAL recovery and rollback retain their required checks.
- Native acceptance requires actual corrector, independent reviewer and QA roots,
  effective role/model/completed-result evidence and digest-bound verdicts. Verify
  source and correcting roots/report link/result remain discoverable after
  reconnect at desktop/mobile widths, with original task output unchanged.
- Run required Phase 2 checks on changed inputs; reuse inspectable unchanged
  receipts only where allowed. Record fixture hashes, commands, bounds, exact
  diffs/preimages/postimages, verdicts and UI/log locators. Implementor publishes
  candidate/evidence → fresh reviewer; supervisor then schedules final QA on
  frozen inputs. Implementor never certifies final acceptance.
- A bounded genuine live GLiDE sample is optional under separate authority;
  report its actual outcome, including no-hit/unavailable/uncertain. No statistical
  calibration claim. Controlled repair acceptance bypasses semantic evaluation:
  **full natural live end-to-end learning remains UNMET** until genuinely shown;
  never relabel a synthetic packet or successful prompt delivery as that proof.

**Rollback:** disable owned disposable loop; restore only matching fixture
postimages, stop only owned bounded services, retain evidence and report conflicts.
Live installation/activation/release/deploy remain outside this plan.

## Execution / evidence discipline

All later commands have GNU `timeout -k` and harness bounds <=600s, file-backed
logs, disposable fixture roots and synthetic canaries. Each phase's packet must
declare its frozen revision, editable paths, evidence root and rollback boundary.
Missing verification stays UNMET. No real secrets/transcripts are needed for
deterministic acceptance. Docs-only checks are whitespace, local-link resolution,
normative-contract consistency and exact changed-file scope, not the full suite.
