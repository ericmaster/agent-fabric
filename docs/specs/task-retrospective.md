---
title: "Spec: Task Retrospective"
type: spec
status: active
covers: internal/retrospective, cmd/agent-fabric (retrospective analyze)
last_checked: 2026-10-06
---

# Task Retrospective — Phase 1 (replay analysis)

Opt-in, harness-neutral, offline replay of one normalized task trace into an
evidence-linked report. Standard library only, no daemon, database, UI, MCP
server, and no mutation outside the requested report file. Native hooks,
dispatch, storage retention, and OpenCode decoding belong to later phases; this
phase owns the shared trace/evaluation/report contract.

Canonical plan: `.artifacts-manager/plan-task-retrospective.md` (Phase 1).
Supervisor persistence is separate: `docs/specs/agent-fabric.md` (supervisor
ledgers are never a retrospective database).

## Files

- `internal/retrospective/config.go` — one config contract (defaults, thresholds, budgets).
- `internal/retrospective/trace.go` — trace schema v1, validation, normalization
  (ordering, dedup, task interval, steps, completeness).
- `internal/retrospective/redact.go` — local redaction before HTTP and before persisted evidence.
- `internal/retrospective/jev.go` — Jev client (HTTP, retries, size-rejection splitting interface).
- `internal/retrospective/evaluate.go` — stage one gate, stage two pagination, counting, triggers.
- `internal/retrospective/report.go` — report assembly and atomic write.
- `cmd/agent-fabric/retrospective.go` — `agf retrospective analyze` CLI wiring.

Every file carries `// Spec: docs/specs/task-retrospective.md`; helpers carry
`// Spec: docs/specs/task-retrospective.md via <parent>`.

## Trace input schema v1 (`--input trace.json`)

Top level:

| field | required | meaning |
|---|---|---|
| `schema_version` | yes | exactly `1` |
| `identities` | yes | `{root_session_id, session_id, execution_root}`; `source_revision`, `task_id` optional |
| `terminal_status` | no | `ok` (default), `aborted`, `error` |
| `task` | no | host task block `{id, start_seq, end_seq?}`; when present it defines the interval and is preferred |
| `boundaries` | no | explicit boundaries; `accepted_success` entries carry `after_seq` (interval = records with `seq > after_seq`); `task_start` entries mark earlier task starts |
| `messages` | no | `[ {id, seq, role: user\|assistant, kind: message\|action\|final\|reasoning\|unsupported, text, hidden?, env?} ]` |
| `calls` | no | `[ {id, seq, tool, status: success\|error\|failed\|aborted\|cancelled\|running ("" = running), exit_code?, input?, input_omitted?, result?, env?, prereq_fingerprint?, source_refs?} ]` |
| `requirements` | no | explicit requirements `[ {id, text} ]`; absence means no omission questions |
| `child_refs` | no | linked descendant session ids that must resolve |
| `descendants` | no | full child traces (same schema, `session_id != root_session_id`); depth ≤ 16 |
| `gaps` | no | collection gaps `[ {kind, detail} ]` |
| `source_files` | no | provenance `[ {path, sha256, excerpt?, secret_file?} ]` |

Validation (fail closed before any network or report write, exit 1):

- `schema_version == 1`; non-empty `root_session_id`, `session_id`,
  `execution_root`; `session_id == root_session_id` for the supplied trace.
- Every message/call/requirement has a non-empty id; a repeated id keeps its
  first occurrence and increments the dedup counter (`stable ... IDs`; ordering
  and deduplication precede step selection and coverage accounting).
- Ordering is stable by `(seq, id)`; all records share one root-level sequence space.
- Statuses outside the enumerated set are validation errors.
- `prereq_fingerprint` is an arbitrary opaque digest recorded by the collector;
  retrospective compares digests for the redundant-attempt exception and never
  invents one when absent.

Task interval resolution:

1. Host `task.id` present → interval `seq >= start_seq` (and `<= end_seq` when
   `end_seq > 0`), boundary source `host`.
2. Else the last `accepted_success` boundary → records with `seq > after_seq`,
   source `accepted-success`.
3. Else the whole session, source `session-start`.
4. More than one `task_start` boundary anchored inside the resolved interval is
   an ambiguous multi-task interval.

Completeness — each material condition makes the interval incomplete (never a
successful complete trace): ambiguous multi-task interval; unresolved running
tool; missing linked child trace (`child_refs` without a matching descendant);
compacted-away evidence (gap `kind: "compacted"`); missing final assistant
response in the interval.

Step definition: a step is a terminal tool attempt with paired input/result
(`input` present, `input_omitted` false, and `len(result) > 0`) or a non-final
visible assistant action message. User messages, the final assistant response,
hidden reasoning, unsupported or binary messages, and unpaired or
`input_omitted` terminal calls are not steps. `last_ten_steps` = the last ten
of the stable ordered deduped steps.

## Redaction (before HTTP and before any persisted derived evidence)

Local-only redactor applies, in order:

1. Env values of records (calls and messages) whose key name matches
   `(?i)(password|secret|token|Api[-_]?Key|Auth|Credential|Passphrase)` → `[REDACTED:env:<name>]`.
2. Secret-file bodies: `source_files` entries and record `source_refs` whose
   path matches secret-file patterns (`.env`, `id_rsa`, `id_ed25519`, `.pem`,
   `.p12`, `.pfx`, `credentials`, `secrets`) or that set `secret_file` have
   their excerpt fully redacted; an unreadable or absent secret-file body
   (empty excerpt) is a materially incomplete redaction and blocks automatic
   correction.
3. Credential/auth/cookie/private-key content patterns (default rails):
   private-key PEM blocks, `Authorization`/`Api-Key`/`X-Api-Key` and
   bearer/basic credentials, cookie values, JSON/string password/secret/token
   assignment, plus every configured sensitive pattern (`redact_patterns`
   config keys, canaries). Patterns are additive; defaults cannot be disabled.
4. Redaction precedes pagination and record splitting: a secret lying on a
   split boundary is already removed, so no half-secret can leak through a
   split. Unsupported, binary, or private-reasoning records are excluded with
   coverage reasons, never transmitted. Unsafe or materially incomplete
   redaction blocks automatic correction in the report.

Redaction counters (per pattern kind) are recorded; never the original values.

## Jev evaluation

- Model identifier is pinned to `jev-1.13.0` in the config contract; the config
  cannot override it. `rubric_version` is the shipped deterministic rubric id.
- `TYPESAFE_API_KEY` is read from the host environment at request time and only
  held in process memory (`Authorization: Bearer ...`); it is never logged,
  written to reports, fixtures, or state. Missing key → evaluator unavailable.
- Endpoint default `https://api.typesafe.ai`; overridable only through the
  config contract (tests point it at a local mock; tests never hit the real API).
- Wire contract: `POST {endpoint}/v1/choices` with JSON
  `{model, state, overlap?, requests:[{id, subject, scope, text, choices:[...], evidence?}]}`;
  response `{responses:[{request_id, subject, choice, labels?, probability, confidence}]}`.
  Stage-two `state.records` carries the page's redacted recordgrams (the
  record content). `overlap` repeats the previous page's tail with
  `overlapped: true` and is the only other place that content appears.
- Both model limits are honored with conservative byte budgets (bytes are not
  claimed to be tokens): serialized request ≤ **24,000 UTF-8 bytes** and
  `state + longest question` ≤ **16,000 bytes**. Measurements include question
  envelopes and recordgrams and happen before any send; a too-large request is
  split, never trimmed.
- Transport failure (HTTP 5xx/network/timeout), HTTP 429 (with Retry-After or
  capped backoff), HTTP 413 (size rejection), and unparseable responses are
  retryable/size-handled per the client contract below; stubborn 4xx failures
  (401/403/404) are permanent failures that stop the run with an incomplete report.

### Stage one — bounded success gate

One request. Evidence contains exactly the three prescribed fields, redacted:

1. `user_messages` — all interval user messages (id + text).
2. `final_assistant_response` — the last visible assistant message inside the
   resolved interval (id + text). Out-of-interval finals, and hidden, binary,
   reasoning, or unsupported records, are not eligible and are never
   transmitted as this task's final.
3. `last_ten_steps` — the last ten steps (id, kind, session/tool/status/
   exit_code/input/result fragments for tool attempts; id/text for assistant actions).

No other records or context. If any of the three fields, or the single outcome
question value, exceeds the 16,000-byte limit or the request exceeds 24,000
bytes, the outcome is `unknown` locally (`oversized_bounded_evidence`) — no
truncation, no extra context, no stage two, and the report records complete
delivery (the choice not to send was local, not a delivery failure).

Outcome choices are exactly `succeeded|incomplete|failed|cancelled|unknown`.
Responsive values arrive as a single choice over the supplied ids with
`probability` and `confidence`. An unknown choice id, missing/out-of-range
probability or confidence, extra/unknown response fields, or an unparseable
response abstains: the outcome records the abstain reason and no stage two is
requested. Stage two is requested only when every gate holds:

- outcome choice `succeeded` with `probability >= 0.90` and `confidence >= 0.80`;
- the interval resolved with complete bounded input (no material completeness reason);
- no terminal abort/error (`terminal_status` is `ok`/absent);
- the evaluator and budget are available.

A locally incomplete interval (missing child trace, running tool, compacted
evidence, ambiguous boundary, missing final response) or a terminal
abort/error is reported `incomplete` with local reasons and never sends any
request for stage one or stage two. A locally-abstained trial (`unknown`
outcome) also sends no stage-two request. Stage-two runs only for confident
`succeeded` outcomes.

## Stage two — paginated classification (only after the success gate)

Questions (each over the exact supplied record ids/requirement ids):

- Per terminal tool call: choose one of
  `productive|necessary-exploration|expected-failure|avoidable-error|wrong-direction|redundant|uncertain`.
- Per explicit requirement: choose one of
  `omitted-high-impact|omitted|present|uncertain`.

Records are the interval's observable records: user messages, visible assistant
action/final messages, terminal tool calls, explicit requirements — including
linked descendants, deduplicated by id once. Hidden reasoning, unsupported, and
binary records are excluded with coverage reasons (never transmitted). Records
larger than a page become stable id-linked fragments
(`part`/`total` on the piece, same record id). Each piece's `piece_source`
is the source segment (`input` or `result:<fragment id>`) that covers the
piece's start offset, not the cut index. `state +
longest question` limits apply to fragment questions. Requests carry `state`
(task/boundary metadata, page number, fragment references), `overlap` (the
previous page's tail records, flagged `overlap: true`), and `requests`.

Pagination:

- Packed deterministically by `(seq, id)` with the two byte limits. Fit checks
  measure the serialized request the way dispatch will send it, including
  question envelopes and recordgrams.
- On a local over-budget measurement or HTTP 413 the page is re-packed with a
  halved page budget until it fits or a single record cannot (then that record
  is unknown `oversized_indivisible` and the rest of the run continues).
  Splitting never omits or trims records. A successful page restores the
  configured budgets for the next page.
- Every record is covered exactly once in coverage accounting; overlap
  repeats are marked and counted once (dedup counter). Overlap re-presents
  the previous page's tail item, id-linked pieces included (marked
  part/total + overlapped).
- Overlap is omitted for a page when it would prevent a record that fits
  alone from being delivered without trimming.
- A record's classification question is sent once, on the page where its last
  fragment appears (after its fragments are reconciled), so cross-page
  recoveries are classified only when the related fragments are visible together.
  Uncertain classifications are never counted.
- HTTP budgets per run: 100 requests (configurable), 30s timeout,
  3 attempts per request with Retry-After or backoff
  (`backoff base 300ms` doubling up to `4s`); `Retry-After` caps at 30s and is
  honored; the INJECTED retry delay is honored in tests.
- Budget exhaustion, repeated 429/unavailable, or a malformed response stops
  the run and saves a report with `status: "incomplete"`, the exact reasons,
  and no correction. Requests are counted across both stages and retries.
- If every stage-two dispatch was size-rejected and no page was delivered,
  the report is `status: "incomplete"` with reason `persistent size rejection`.
  Per-record unknowns stay recorded; the incomplete reason distinguishes a
  persistent server size fault from a record that was locally oversized.

## Counting, rate, and trigger

- `counts.terminal_calls` N = unique terminal calls (compound commands count
  once per call id; duplicate events and overlap repeats do not inflate).
- Per call at most one confident classification; records with no response or
  an invalid response are `uncertain` (with reason) and excluded from the
  avoidable numerator but kept in the denominator, so the rate stays a bound
  (the report records `bounded: true`).
- Confident avoided waste (`avoidable-error`, `wrong-direction`, `redundant`)
  needs `probability >= 0.90` and `confidence >= 0.80`; dedup multiple waste
  labels per call id (union counted once).
- `counts.raw_errors` counts terminal calls whose status is `error`, `failed`,
  or `aborted`, and terminal calls with a recorded nonzero exit code, split
  into `recovered_or_expected` / `avoidable` / `unclassified` so recovered and
  expected red-test/no-match outcomes stay distinct from waste. The superset
  (aborted status and nonzero exit, not only `error|failed`) is intentional.
- Required-step omissions from requirement verdicts `omitted` /
  `omitted-high-impact` (confident) are reported separately, never folded into
  the call denominator.
- Rate = confidently avoidable unique calls / all terminal calls; zero calls
  means no rate trigger. Trigger is eligible when `N >= 20` and `rate > 0.05`
  (exactly 5% does not trigger), or when three verified redundant attempts share
  unchanged prerequisite fingerprints (all supplied digests equal), or when one
  requirement is confidently `omitted-high-impact`. Exceptions hold with
  fewer than 20 calls.
- `thresholds` in the report expose the exact used values.

## Report schema v1 (`--output report.json`)

- `schema_version: 1`; `generated_at` (UTC RFC 3339).
- `status` `complete|incomplete` + `incomplete_reasons[]`;
  `model: {model: "jev-1.13.0", rubric: "task-retrospective-rubric-1"}`.
- `trace` — source identities, boundary resolution, gap redaction (gap `detail`
  and other collector free text pass through the redactor before persistence),
  input `sha256`, `source_files`
  (path + sha256; secret-file bodies never persisted), `child_refs`,
  coverage of descendant sessions included in the interval, digest of records included,
  and namespace dedup counters.
- `stage_one` — payload field/step counts, whether the payload was sent,
  the outcome choice (+ probability/confidence when modeled), and the exact
  gate verdicts behind stage-two dispatch.
- `evaluation` — per-call classifications `{subject, page, choice, labels?,
  probability, confidence, counted, reason?}`, requirement verdicts,
  the counter block (avoided/uncertain/raw errors/requirements...), the
  computed rate (bounds + trigger), and the trigger decision with thresholds.
- `coverage` — per-request page coverage (`pages`, `record_dispositions`),
  and per-record dispositions (`covered`, `skipped: <reason>`), boundary/child
  digests, record dedup counters.
- `redaction` — pattern list, counts (substitutions, partial/env/secret counts),
  and the `correction_safe` verdict (false when materially incomplete).
- `usage` — HTTP requests used, retries, splits (content fragmentations plus
  size-rejection re-packs), `budget` size, exhausted flag.
  `evaluation.stage_two_ran` is true only when at least one stage-two page
  was delivered.
- `dispatch` — `eligible` (an eligible trigger + complete coverage + safe
  redaction), reasons when not eligible. Stage two eligibility additionally
  requires every page delivered. Materially incomplete redaction (an excluded
  binary record, or a secret-file ref with an empty/unreadable body) sets
  `eligible` false with an explicit reason.

Report files are written atomically (temp file + rename in the output directory)
with mode 0600. A report is ALWAYS written for a structurally valid trace even
when the outcome is unknown or evaluation is incomplete.

## CLI

```
agf retrospective analyze --input trace.json --output report.json [--config task-retrospective.config.json]
```

- Exit 0: report written (any report status).
- Exit 1: unreadable/malformed trace input, missing/invalid `--input`/`--output`,
  invalid config, or another pre-delivery failure; no report is written.
- Trace JSON is decoded strictly (`DisallowUnknownFields`). Unknown fields or
  trailing values fail closed before any network call or report write.
- Config JSON uses the same fail-closed decode: unknown fields or a trailing
  value after the config object are rejected.
- The host supplies `TYPESAFE_API_KEY` from the environment; it is never logged
  or persisted. Config never contains credentials.

## Config contract (one place)

`internal/retrospective/config.go` owns all defaults and thresholds:
`model`, `rubric_version`, `endpoint`, `http_timeout` (`"30s"`),
`maximum_attempts` (3), `request_budget` (100), `page size budget_bytes`
(24,000 request / 16,000 state+longest), `minimum fragment bytes` (1,024),
`backoff base/max` (`"300ms"` / `"4s"`), `retry-after max` (30s), `minimum success
probability` (0.90), `minimum confidence` (0.80), `minimum calls for rate`
(20), `rate threshold` (0.05 exclusive), `redundant attempts required` (3),
`steps tail size` (10), and additive `redact_patterns`.
Duration fields (`http_timeout`, `backoff_base`, `backoff_max`) are
unit-suffixed strings. A bare number is rejected; it is not interpreted as
nanoseconds.
No other file duplicates these defaults; later phases read the same contract.

## Testing contract

- Unit tests in `internal/retrospective` and `cmd/agent-fabric` cover the
  classification/rates/stage-two paging/redaction contracts with recorded mock
  HTTP (`httptest` servers) and synthetic traces only.
- Fixtures are synthetic and disposable; they may only contain synthetic
  secrets (`CANARY-*` strings). Tests prove canaries never reach HTTP bodies,
  persisted reports, or logs.
- `go test ./internal/retrospective/... ./cmd/agent-fabric/...` plus the repo
  regression `go test ./...`, `go build ./cmd/agent-fabric`, and
  `python3 -m unittest discover -s tests -p 'test_supervisor_ledger.py'` stay green.
