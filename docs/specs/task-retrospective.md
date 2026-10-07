---
title: "Spec: Task Retrospective"
type: spec
status: active
covers: internal/retrospective, cmd/agent-fabric (retrospective), hooks/retrospective, agents/learning-corrector.md
last_checked: 2026-10-07
---

# Task Retrospective

Opt-in, harness-neutral replay of one normalized task trace into an
evidence-linked report, then one visible correcting root for an eligible report.
Standard library only. No daemon, database, new UI, or MCP server. Phase 1
writes only the requested report file. Phase 2 adds dispatch, local state, and
verified text repair. Phase 3 ships the OpenCode observation plugin, staged
event spools, and the detached drain worker that runs the complete loop
(observe → ingest → analyze → dispatch) under the global opt-out. Supervisor
ledgers are not a retrospective database.

Canonical plan: `.artifacts-manager/plan-task-retrospective.md` (Phase 1).
Supervisor persistence is separate: `docs/specs/agent-fabric.md` (supervisor
ledgers are never a retrospective database).

## Files

- `internal/retrospective/config.go` — one config contract (defaults, thresholds, budgets).
- `internal/retrospective/trace.go` — trace schema v1, validation, normalization
  (ordering, dedup, task interval, steps, completeness).
- `internal/retrospective/redact.go` — local redaction before HTTP and before persisted evidence.
- `internal/retrospective/jev.go` — GLiDE client (legacy Go name; HTTP, retries, size-rejection splitting).
- `internal/retrospective/evaluate.go` — stage one gate, stage two pagination, counting, triggers.
- `internal/retrospective/report.go` — report assembly and atomic write.
- `internal/retrospective/store.go` — local state, locks, retention, settings entry.
- `internal/retrospective/dispatch.go` — session dispatch, canonical resolution, repair.
- `internal/retrospective/observe.go` — OpenCode observation envelope validation, staged spool, ingest, trigger.
- `internal/retrospective/drain.go` — persisted drain jobs, spool→trace assembly, server backfill, detached worker.
- `hooks/retrospective/opencode.ts` — thin OpenCode observation adapter (plugin asset).
- `cmd/agent-fabric/retrospective.go` — `analyze`, `dispatch`, `status`, `enable`, `disable`, `ingest`, `drain`.
- `agents/learning-corrector.md` — portable public primary correcting role.
- `assets/retrospective/learning-origin.json` — optional owned runtime asset.

Every Go/Terminal file carries `// Spec: docs/specs/task-retrospective.md`;
helpers carry `// Spec: docs/specs/task-retrospective.md via <parent>`. The
TypeScript adapter carries the equivalent header comment.

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

## GLiDE evaluation

The default evaluator is Fastino GLiDE. Authoritative contracts (checked
2026-10-07): [System One](https://docs.fastino.ai/inference/systemone) and
[Choice](https://docs.fastino.ai/concepts/glide-choice), plus the
[OpenAPI schema](https://docs.fastino.ai/openapi.json) (`ChoiceQuestion`
criteria minProperties=1; `SystemOneRequest` questions minProperties=1).
`evaluator: "glide"` pins `fastino/GLiDE`, endpoint `https://api.fastino.ai`,
and host environment `FASTINO_API_KEY` sent only in `X-API-Key`.
Jev is reserved as an explicit opt-in profile, but selecting it currently
fails configuration validation: its current pinned-model contract has not
been independently verified. No invented Jev protocol or fallback is sent.
Internal legacy `JevClient`/`ChoiceRequest` names are compatibility names,
not provider claims.

GLiDE wire: `POST /v1/systemone`, JSON `{model,state,questions}`;
`questions` is keyed by supplied IDs, values are `{type:"choice",instructions,
criteria:{choice_id:description}}`. Instructions explicitly identify the
subject ID and scope. Question evidence is folded into `state.evidence[id]`,
overlap into `state.overlap`; encode failures fail closed, never drop evidence.
Stage one state contains ONLY `evidence.q-outcome` (the prescribed three
fields), no root/session/path/revision metadata. Stage two retains its
redacted page records and context. Fragment-only pages with no classification
question receive one transport-only `q-context-page` Choice with the sole
criterion `context-only`; its acknowledgement is ignored by the normalized
core, never counted or attributed. This satisfies GLiDE's non-empty question
map without inventing a classification on incomplete fragments.
Response `{model:"glide",answers}` uses
exact matching question IDs and Choice objects `{type,choice,confidence,
probabilities}`. Documented `usage` and `token_usage` metadata are accepted
without attribution and are not persisted; other unknown fields abstain.
Model/type/IDs/choice and a full probability distribution are mandatory:
every supplied choice exactly once, no extra keys, all values present,
non-null, finite and in [0,1], sum within 0.00001 of 1, winning choice maximal.
Confidence must be present, finite and in [0,1]. Duplicate JSON keys and
response nesting beyond 128 levels are rejected. Malformed responses retry
within the same budget; exhaustion yields unknown/incomplete and no repair.

Each rendered state+question is limited to 40,000 tokens, each Choice to
255 options. We conservatively cap actual serialized wire at 24,000 UTF-8
bytes and folded state+longest serialized keyed wire question at 16,000 bytes
(not a token-count equivalence). Limits may only be lowered. The same
wire encoder measures packing, stage one, and the final client pre-send
gate; criteria and folded evidence/overlap count. No trimming is permitted.
Retries include 425/429/503 and transport/5xx errors with bounded backoff;
Retry-After is honored on all these statuses, capped at configured 30s.
The existing 30s per-request timeout remains configurable; Fastino recommends
at least 300s for live read timeouts. No redirects are followed (API keys
cannot propagate). Responses over 4 MiB are detected, not truncated.
The .90/.80/N>=20/rate>5% gates below are unchanged engineering defaults,
not GLiDE calibration claims; live calibration and visible-root QA are unmet.

### Provider-independent core

- Model identifier is pinned to `fastino/GLiDE` in the config contract; the config
  cannot override it. `rubric_version` is the shipped deterministic rubric id.
- `FASTINO_API_KEY` is read from the host environment at request time and only
  held in process memory (`X-API-Key`); it is never logged,
  written to reports, fixtures, or state. Missing key → evaluator unavailable.
- Endpoint default `https://api.fastino.ai`; overridable only through the
  config contract (tests point it at a local mock; tests never hit the real API).
- Normalized internal contract remains `ChoiceRequest`/`ChoiceResponse`;
  HTTP uses the GLiDE wire translation specified above, not that internal JSON.
  Stage-two `state.records` carries the page's redacted recordgrams (the
  record content). `state.overlap` repeats the previous page's tail with
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
(task/boundary metadata, page number, fragment references), `state.overlap`
(the previous page's tail records, flagged `overlapped: true`), and keyed
`questions` on the wire.

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
  `model: {model: "fastino/GLiDE", rubric_version: "task-retrospective-rubric-1"}`.
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
- The host supplies `FASTINO_API_KEY` from the environment; it is never logged
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
No other file duplicates these defaults. Phase 2 reads this struct and adds the
dispatch keys in the Phase 2 config section. `evaluator` defaults to `glide`;
`jev` is rejected pending verified contract. The pinned evaluator model is not a
config key; a `model` key remains unknown.

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
- Phase 2 session tests use `httptest` only. They never dial a real OpenCode server.

## Phase 2 — dispatch and verified text repair

Turn one eligible report into one visible correcting root that either applies a
verified text repair or records a review-only suggestion. Disabled is the
default and performs no network call and no mutation.

### Global enable

`retrospective.enabled` lives in the user Agent Fabric config
(`AGF_CONFIG`, else `$XDG_CONFIG_HOME/agent-fabric/config.json`, else
`~/.config/agent-fabric/config.json`) as:

```json
{"retrospective": {"enabled": false}}
```

Absent file or absent key means false. `enable` and `disable` rewrite only
that key. Other top-level keys and sibling keys inside `retrospective` are
preserved. This is not a shared-settings takeover. The analyze config file
cannot enable dispatch.

### Dispatch config keys

Added to the same `Config` struct, not a second defaults table:

| key | default | meaning |
|---|---|---|
| `dispatch_server` | empty | OpenCode server base URL. Empty queues; it is not the evaluator `endpoint`. |
| `dispatch_server_username` | empty | HTTP Basic username for the session transport; empty falls back to `OPENCODE_SERVER_USERNAME`, then `opencode`. |
| `dispatch_server_password` | empty | HTTP Basic password; empty falls back to the server env (`OPENCODE_PASSWORD`, then `OPENCODE_SERVER_PASSWORD`). Sent only as the Authorization header; never logged or persisted. |
| `dispatch_ui_template` | empty | `http` or `https` URL with exactly one `{id}`, no userinfo. |
| `dispatch_model` | empty | Optional host model string sent to the server. Never copied into canonical agent text. |
| `dispatch_directory` | empty | Directory recorded on the root. Falls back to the report execution root. |
| `dispatch_timeout` | `"30s"` | Per create/read/list/prompt timeout. Unit-suffixed string. |
| `canonical_roots` | empty | Absolute editable roots. No project or skill enrollment list. |
| `reviewer` | empty | Coordinator-supplied independent reviewer. Must not be `learning-corrector`. |
| `state_dir` | empty | Overrides the user state directory. |
| `max_repair_attempts` | 2 | One initial correcting run plus one repair attempt per finding/source version. |

A canonical root of `/`, a relative path, a symlink, or a path that does not
exist is unsafe and rejected at validation. Config JSON rejects a trailing
value after the object, the same way trace and report JSON do. There is no
per-source human approval gate.

### State

Default directory: `AGF_RETROSPECTIVE_STATE`, else
`$XDG_STATE_HOME/agent-fabric/retrospective`, else
`~/.local/state/agent-fabric/retrospective`. Directory mode `0700`, file mode
`0600`, writes are temp-file plus rename. A single-host exclusive lock covers
claim and session creation. This directory is not a supervisor ledger.

Redacted report and trace copies under `reports/` and `traces/` are deleted
after 30 days. Compact finding/source-hash records and dispatch receipts are
kept until that source hash changes. `--retry` does not create a second root
for the same report and task, does not re-apply a successful repair of the
same source hash, and does not post again after an ambiguous create or prompt.
A finding that already used `max_repair_attempts` is exhausted. A new report
for the same finding and source hash may `--retry` once more until that cap;
the next attempt is exhausted and does not write. A changed source hash is a
new incident. A second dispatch of a repaired report returns the stored
receipt and leaves the file untouched.

A queued receipt is not a dead end. If create was never attempted and
`dispatch_server` plus `dispatch_ui_template` are now set, a later dispatch
persists the marker, records the create attempt, and creates the first root.
If create was attempted and no root id was read back, a later dispatch without
`--retry` stays queued and does not post. `--retry` lists sessions and adopts
one exact marker match; it does not call `POST /session` again. Zero or many
matches stay pending.

### Learning-origin identity

Format: `agf-learning-origin:v1:` plus 64 lowercase hex characters, the SHA-256
of `learning-origin\n` concatenated with the incident id. The incident id is
the SHA-256 of `report-file-sha256\n` concatenated with the task id.

The marker file is persisted before `POST /session`. The session title is
`learning: <task> [<marker>]`. The task id is truncated to 48 characters; the
marker is not truncated. After read-back, `origins/<session-id>` stores the
marker. The store is authoritative. The title is the reconciliation echo.

Phase 3 capture must ignore a session whose id is in `origins/` or whose title
contains `agf-learning-origin:v1:`, and every descendant whose parent chain
reaches such a session. Worker sessions created here carry the marker.
Dispatch does not capture them.

### OpenCode session transport

Fixture-tested HTTP, no other UI:

- `POST /session` body `{"title","directory"}`. The `parentID` key is absent.
- `GET /session/{id}` must echo the marker in `title` and return an empty
  `parentID`. A non-empty `parentID` is rejected.
- `GET /session` returns a JSON array used only to reconcile a timeout.
- `POST /session/{id}/prompt` body `{"parts":[{"type":"text","text":"..."}],"marker","agent":"learning-corrector"}`.
  Optional `model` is included only when `dispatch_model` is set.
- HTTP 404 or 501 queues the report with an actionable error. No child session
  and no CLI fallback.
- A timeout lists or reads before any retry. Zero or many marker matches stay
  `pending`. An ambiguous create or prompt is not posted again.

The receipt records root id, project, directory, title, UI URL from the
validated template, report locator, and status
(`queued`, `pending`, `submitted`, `repaired`, `conflict`,
`exhausted`, `suppressed`). After a confirmed prompt, advisory findings use
receipt status `submitted`. `suggested` is a finding action, not that receipt
status.

### Canonical resolution

Follow Fabric manifest ownership, `.skills-manager/ownership.json`, and
symlinks. Edit only the owning canonical file inside a canonical root.
Generated copies and skill-manager caches are not edited. `immutable: true`
or a directory `.immutable` marker produces a suggestion to update the upstream
source. Two different canonical owners are an ambiguity and are rejected.
A symlink whose target leaves the allowed roots is rejected. Resolution
errors, including an unsafe or missing root, reject the dispatch before any
session is created. They are not recorded as a per-finding conflict.

Skill ownership shape:

```json
{"schema":1,"skills":{"name":{"canonical":"/abs/SKILL.md","installed":"/abs/cache/SKILL.md","immutable":false}}}
```

### Repair

`findings[]` on a report are optional dispatch evidence. Analyze does not emit
them. A finding is applied only when the cause is `stale-instruction`, the
instruction version equals the source hash, failed behavior and recovery are
both present, the source was not merely read, an independent reviewer is
configured, the excerpt is unique, and the on-disk hash still matches
immediately before the write. The write replaces that excerpt only, preserves
mode, and refuses symlinks and executable files.

Other causes do not write:

- `tool-bug` and `implementation-bug` — review-only suggestion. No documentation workaround.
- `instructions-ignored` — workflow suggestion.
- `transient` and `uncertain` — advisory suggestion.
- `prompt-injection`, or injection text in the finding or replacement — no write.
- hash mismatch — conflict; unrelated bytes stay.
- non-owned or immutable — suggestion to update upstream.

Every result records the reviewer and `self_certified: false`. A repair records
the exact excerpt diff, preimage hash, postimage hash, and a `0600` preimage.
If validation fails and the current hash is still the postimage, only that
repair is restored. A later user edit is a conflict, not an overwrite.

Protected paths (learning policy, the user config file that owns
`retrospective.enabled`, any explicit settings path, hook trees, origin
markers, state files, executables) are never edited. Transcript text cannot
authorize those edits. Dispatch does not commit, push, force-sync, or change
permissions. A failed finding-record write is appended to that finding's
suggestion and does not hide the repair result.

After a validated repair of a canonical agent definition, unchanged
manifest-owned generated copies may be refreshed. The CLI passes manifests
discovered under the canonical roots and the source root. Copies whose bytes
no longer match the manifest hash are left in place and reported. A failed
manifest hash update is a copy note with action `manifest-update-failed`.

### Correcting role

`agents/learning-corrector.md` is a public primary role on the existing
`worker` profile. The body is provider-neutral. It validates a self-locating
intake packet before edits, treats the correction as a new scope, and does not
change the original task acceptance state. The intake prompt carries the report
locator, finding summary, source identity and hash, verified behavior, and the
prohibitions above.

### CLI

```
agf retrospective dispatch --report report.json [--config file] [--retry]
agf retrospective status [--config file]
agf retrospective enable
agf retrospective disable
```

`dispatch` validates report schema, `evaluation.trigger.eligible`,
`dispatch.eligible`, complete coverage (`undelivered_records == 0`),
`redaction.materially_incomplete == false`, task id, execution root, and source
hashes before any session call. Exit 0 when a receipt is written, including
queued and pending. Exit 1 when disabled, the report is ineligible or malformed,
or config is invalid. No receipt is written in those exit-1 cases.

### Optional asset

Resource identity `retrospective-learning-origin` is not an agent id. Source
discovery reads `assets/retrospective/learning-origin.json` from the packaged
source root. `install`/`sync --assets retrospective` owns
`<scope>/retrospective/learning-origin.json` through the manifest, using the
existing guarded write. Omission is opt-out. Doctor, sync, and uninstall keep
modified copies and remove only unchanged owned copies.

### Phase 2 tests

- Disabled dispatch makes no client call and creates no state or source file.
- Duplicate and concurrent dispatch create one root.
- A lost create or prompt response reconciles from the server and does not post again.
- Unsafe roots, escaping symlinks, and canonical ambiguities are rejected.
- Stale-runbook and shared-skill fixtures apply a minimal repair without a prompt.
- Tool-bug, non-owned, instructions-ignored, dirty-target, and prompt-injection fixtures do not write.
- Reviewer and rollback evidence are recorded. Validation failure restores only the repair.
- Install, sync, doctor, and uninstall cover the optional asset.

## Phase 3 — OpenCode observation and the complete learning loop

The observation loop is: OpenCode emits native bus events → the thin
TypeScript adapter (`hooks/retrospective/opencode.ts`) converts them to a
versioned observation envelope → the adapter hands the envelope to the
packaged Go CLI over stdin (`retrospective ingest --harness opencode`) →
ingest stages bounded records in a local spool → a terminal trigger persists
a drain job and launches one detached drain worker → the worker reconciles
the terminal snapshot against the server session API, assembles trace schema
v1, runs `analyze`, and dispatches eligible reports. The plugin never
interpolates transcript data into shell arguments, never posts into the
source session, and never blocks its response on evaluation: observation is
fire-and-forget staging plus a detached worker.

### Installed version pins

- Observed pairing for fixtures and runtime: OpenCode CLI `2.0.24` with
  `@opencode-ai/plugin@1.18.35` and `@opencode-ai/sdk@1.18.35` (the npm
  generation that matches the installed CLI; the planning note recorded
  `1.18.32`, since superseded by the installed `2.0.24`).
- The adapter is version-tested: it probes the server version once and stays
  inert (logs degradation, never stages) for a version it cannot confirm. SDK
  shape differences are normalized inside the adapter only; the Go core knows
  only the envelope below.

### Observation envelope v1 (plugin → ingest)

One flush carries one JSON document over stdin:

| field | required | meaning |
|---|---|---|
| `version` | yes | exactly `1` |
| `harness` | yes | exactly `opencode` |
| `session_id` | yes | the owner session of this flush |
| `server_url` | no | observed server base URL (http loopback or configured host), used only by drain backfill |
| `sdk_version` | no | adapter-observed SDK/app version stamp, recorded for audit |
| `trigger` | no | true when this flush is a terminal trigger candidate |
| `records` | yes | bounded record array (≤ 2,000 records) |

Every record is one of the following kinds (single object, `kind` selects):

- `session` — `{session_id, parent_id?, title?, directory?}`; identity and
  parent linkage observed from session events.
- `message` — `{record_id, session_id, role}`; role map (`user`/`assistant`).
- `part` — `{record_id, session_id, message_id, part_type, text?, synthetic?,
  reasoning?, tool?, tool_status?, call_id?, input?, output?, error?}`.
  Reasoning parts are emitted with `reasoning: true` and no text: private
  reasoning is never collected.
- `removed` — `{record_id, session_id, message_id?}`; history changed under
  observation.
- `event` — `{event, session_id, status?, detail?}`; observed
  `session.idle`, `session.status` (busy/idle/retry), `session.error`
  (detail = bounded error type name, never a full stack), or
  `session.compacted`.
- `source` — `{session_id, path, sha256, excerpt?}`; content hash captured at
  use time (the file bytes as the adapter hashed them when the read tool
  returned).
- `source_gap` — `{session_id, path, detail}`; a load the adapter saw but
  could not hash.

Ingest performs only these operations: validate (strict decode; unknown
envelope fields or a `version != 1`, `harness != opencode`, oversized batch
(> 8 MiB body), or malformed record fails closed with exit 1), check the
global enable (exit 2, no writes, when disabled), check learning-origin
suppression and the scoped disable override, stage the spool, and on a
trigger write/persist a drain job and launch the detached worker. Exit codes:
`0` staged, `1` ingest error, `2` disabled (not staged), `3` unsupported
adapter version. Stdin is the only transcript channel; argv carries static
flags only.

### Learning-origin suppression (observation side)

A session is excluded from capture when: its id is stored under the dispatch
`origins/` directory; its observed title contains the learning-origin marker
prefix (`agf-learning-origin:v1:`); or a parent chain built from observed
session records reaches such a session. A scoped disable override applies
when the observed session directory or an ancestor contains a
`.retrospective-disabled` marker file. Suppression is evaluated per envelope;
a session that becomes suppressed mid-stream has its spool file removed and
no jobs are ever created for it.

### Staged spool format

`<state>/spool/<session-id>.json` (schema `1`), mode `0600` under the `0700`
state directory, written temp-file + rename behind the shared `spool.lock`:

| field | meaning |
|---|---|
| `session_id` | owner session |
| `last_seq` | monotonic per-record sequence counter (first observation order) |
| `accepted_after_seq` | accepted-success boundary: when > 0, the next task interval starts strictly after it |
| `overflows` | count of envelopes dropped by the spool bound |
| `sessions` | observed `{parent_id, title, directory}` per session id |
| `messages` | role per message id |
| `parts` | latest observed part state per part id (upsert, streamed updates replace) |
| `events` | append-only bounded session events (last 400) |
| `source_refs` | deduplicated `{path, sha256, excerpt?}` provenance |
| `gaps` | bounded collection gaps (`compacted`, `partial_stream`, `missing_source`, overflow) |
| `suppressed` | true once the session matched a suppression rule |

Bounds are enforced at ingest: envelope ≤ 2,000 records, each record field
≤ 64 KiB (clipped to the nearest rune start, so truncation never produces
replacement characters), spool size ≤ `spool_max_bytes` (default
2,000,000). Over the spool bound the remainder of that envelope is dropped
with a `partial_stream` gap and an `overflows` increment; backfill may
restore the evidence later. A bound refusal that leaves fixed metadata alone
above the bound discards the in-memory spool and keeps the last persisted
document: the refusal is the observable record, not an unpersisted counter
bump, so memory and disk stay consistent.

### Event → trace normalization mapping (drain, Go side)

| observed | trace record |
|---|---|
| user text parts of a message | one `messages[]` record, role `user` |
| assistant text parts of a message | one `messages[]` record; the last assistant message of the resolved session carries kind `final` |
| reasoning parts | one `messages[]` record kind `reasoning`, no text (excluded from evidence with coverage reason) |
| synthetic/ignored parts | not captured (not agent observables); their absence is not a gap |
| tool part (`state.status` pending/running) | `calls[]` record status `running` |
| tool part (`state.status` completed) | `calls[]` record status `success` |
| tool part (`state.status` error) | `calls[]` record status `error` with `error` detail in the input fragment set |
| tool state input (before-hook or part state) | `input` JSON string (bounded) |
| tool state output (after-hook or part state) | `result` fragment (`f:<partID>`) |
| tool state error | recorded as call `error` status detail, never as trace `terminal_status` |
| `session.error` event after the last user message | trace `terminal_status` `error` (transient/transport layer), detail `MessageAbortedError` → `aborted` |
| `session.error` event before a later user message | turn-level transport failure; not the trace terminal status |
| `session.compacted` event | gap `kind: compacted` |
| removed records | gap `kind: partial_stream` |
| overflow | gap `kind: partial_stream` (detail names the bound) |
| unhashable instruction source | gap `kind: missing_source` |
| session record with `parent_id` (child) | `child_refs` entry + descendant trace (built from that child's staged records/backfill) |
| every user message after the first | boundary `task_start` at its seq |

Instruction-source capture: read-family tool calls (`read`) whose input
references a known instruction basename (`AGENTS.md`, `CLAUDE.md`,
`CONTEXT.md`, `SKILL.md`) or an instruction directory (`/docs/`, `/skills/`,
`/.agents/`, `/docs-context/`) produce a `source_refs` entry when the
adapter could hash the bytes at use time; otherwise a `missing_source` gap.
Exit codes are recorded only when the tool payload makes them observable;
nothing is inferred. No requirements are synthesized; explicit requirement
records are always host-supplied.

Boundaries and terminal status: the interval resolves per trace schema v1 —
accepted-success boundary when `accepted_after_seq > 0`, otherwise
session-start. Task identity when no host task id exists:
`retro:<session-id>:<first user message seq>:<final message id>` — stable
across restarts so an identical completed turn never produces a second
incident for the same task half.

### Drain jobs and lifecycle

`<state>/jobs/<session-id>.json` (schema `1`): `{session_id, server_url?,
status: pending|running|done|failed|skipped, attempts, incidents:{task:
receipt status}, report_path?, receipt_status?, error?, created_at,
updated_at}`. Skipped covers observation rows that never trigger (a child
session's terminal event, learning-origin rows, suppressed spools).
Ingest writes/persists a job at a terminal trigger (trigger candidate =
`session.idle` or `session.status: idle` for the owner session) with
`status: pending`, then launches one detached `agf retrospective drain`
worker. The worker:

- takes a non-blocking single-host `drain.lock`; a second worker exits
  immediately (never duplicates work);
- iterates pending jobs oldest-first under `drain_max_jobs` per pass,
  re-scanning after the last pass to pick up jobs created while it ran
  (bounded re-check), then exits — there is no always-on daemon;
- per job: reconciles the terminal snapshot from the server when
  `server_url` is known (the owner's history fetched exactly once per job;
  every spool load+stage+write — owner and children — holds that session's
  spool lock in drain.lock → spool.lock order, the same lock ingest holds,
  so a racing plugin flush can never lose staged records; the only other
  lock order is ingest's spool-lock-only, so no deadlock is possible.
  Each level's lock is released before descending into children, and a
  visited set plus the 16-deep descendant budget terminate the walk even on
  a cyclic observed-parent topology; marking a `partial_stream` gap when
  the reconcile fails — transport errors are recorded as gaps, never
  invented as coverage);
  assembles the trace; validates; runs `analyze` (evaluator budgets per config);
  writes the report into `<state>/reports/`; runs `dispatch` under the same
  global-enable re-check and request budgets; records the receipt status in
  the job and updates the spool's accepted-success boundary after a
  conclusive disposition (repaired/submitted/conflict/exhausted/suppressed
  or a complete report that does not trigger);
- disabled mid-flight (global opt-out) → dispatch is skipped and the job
  stops at `pending` (paused work stays inspectable; the next trigger or
  restart resume continues it);
- restart resume: a pending/running job left behind by a crashed worker is
  resumed by the next worker (or the next trigger); `attempts` increments;
  `done`/`failed` jobs are terminal and not re-run.

### CLI

```
agf retrospective ingest --harness opencode            # envelope JSON on stdin
agf retrospective drain                                # worker: process pending jobs
```

`ingest` exit codes and behavior are fixed above (2 = disabled, no writes;
0 = staged, `--json`-style one-line summary on stdout). `drain` exits 0 when
it processed/resumed jobs (each job outcome is inside its job file), exit 1
on lock-unavailable-owner errors; a failed lock because another worker holds
it exits 0 without work. Config resolution for ingest/drain: `--config` flag,
else `AGF_RETROSPECTIVE_CONFIG`, else
`$XDG_CONFIG_HOME/agent-fabric/retrospective.json`, else defaults — so the
detached worker inherits the same evaluator/dispatch configuration the user chose.

### Optional trusted coordinator packet

Dispatch evidence in the loop is an optional
`<state>/coordinator/<task-id>.json` packet (file name = the safe spool-key
form of the task id) — `{schema: 1, findings: [...]}` in the exact Phase 2
`Finding` schema — staged by a trusted coordinator before the drain dispatch
(an automatic loop has no self-certifying author: the packet is external
evidence, the same way `dispatch --report` consumers supply findings today).
The drain validates the packet under exactly the dispatch gates (hash
consistency, canonical resolution, reviewer, excerpt uniqueness, on-disk
hash re-check) and only then includes it in the report written for that
task. Absence is the normal path: the visible correcting root receives the
intake locator and performs its own verified review in its own session;
queued receipts block the task for manual reconciliation.

### Dispatch transport generation

`dispatch_endpoint` config key picks the OpenCode server generation:

- `prompt` (default) — the Phase 2 contract exactly: `POST /session` body
  `{title, directory}` (no `parentID` key), prompt `POST /session/{id}/prompt`
  with the marker echo, reconcile by session read.
- `message` — the verified live 2.x contract (empirically confirmed against
  an isolated OpenCode 2.0.24 server; recorded in
  `fixtures/retrospective/opencode/live-api-2.0.24.json`): every route is
  served under the `/api` prefix; requests carry HTTP Basic auth when
  credentials resolve (see below); `POST /api/session` body `{title}` returns
  a `{"data": …}` envelope (the project directory comes from the server's
  working directory); the session list and per-session history are
  `{"data": …, "cursor": {next}}` pages the clients follow to exhaustion
  (bounded; a page limit fails closed); prompt delivery uses the synchronous
  `POST /api/session/{id}/prompt` with the flat body
  `{"text": …, "agent": "learning-corrector"}` — the 2xx user-message echo
  confirms delivery without requiring a model, and the marker stays in the
  prompt text; reconcile of a lost prompt searches the flat
  `{id, time, text, type}` history rows for the marker. 404/501 queue the
  report exactly as before; no hidden fallback.

**Authentication.** The live server requires HTTP Basic auth on every `/api`
route (a server started without a password generates a random one, which the
loop cannot learn — such servers report the capability gap and stay queued).
Credentials resolve in order: `dispatch_server_username` /
`dispatch_server_password` config keys, else the OpenCode server process
environment (`OPENCODE_SERVER_USERNAME`; `OPENCODE_PASSWORD`, then
`OPENCODE_SERVER_PASSWORD` — the names the server itself reads; the drain
worker inherits the server env). An empty username with a non-empty password
uses the documented server default `opencode`. Credentials are only ever sent
as the Authorization header — never logged, persisted, or echoed in receipts.

Both generations refuse a returned child session (`parentID` non-empty) and
both reconcile a lost create by exact marker list match. The receipt contract
is unchanged.

### Plugin install and activation

The plugin is an opt-in owned asset, never a default install:

- Resource identity `retrospective-opencode-plugin` (not an agent id) in the
  manifest; `--assets retrospective` installs both `learning-origin.json`
  (existing) and `retrospective.ts` (the packaged `hooks/retrospective/opencode.ts`).
- Destination under the OpenCode plugin path for the selected scope: global
  `<home>/.config/opencode/plugins/retrospective.ts`, project
  `<project>/.opencode/plugins/retrospective.ts`. The adapter file is
  self-contained (no runtime imports beyond Bun/Node built-ins) so no project
  `package.json` is required.
- Activation paths: installed plugin file + one global `retrospective.enabled`
  setting. The scoped disable override (`.retrospective-disabled` marker) is
  honored by ingest. Uninstall/sync/doctor keep modified plugin files and
  remove only unchanged owned copies, exactly like the other optional assets.
- Existing live sessions are never activated by install or tests: fixture
  tests only point disposable config/data directories at the plugin path.

### Phase 3 tests

- `hooks/retrospective` npm tests replay recorded OpenCode `2.0.24` event
  fixtures (offline, deterministic) through the real adapter module; the CLI
  spawn path is exercised with a fixture CLI captured by the test.
- Go replay tests replay the same fixtures through `ingest` covering
  streamed updates, duplicate/out-of-order terminal events, root/child
  aggregation, abort/error-then-idle, multi-turn boundaries, history
  compaction, restart resume, learning-session suppression,
  concurrent-ingest-vs-backfill serialization (no lost staged records),
  cyclic-parent backfill termination (visited set + depth budget),
  rune-safe truncation (no replacement characters, whole first rune under a
  sub-limit bound), and bound-refusal memory/disk consistency (both the
  fixed-metadata-alone and the gap-note-drop sites).
- Loop e2e tests run ingest → drain → analyze (mock GLiDE endpoint) → dispatch
  (mock session server) and prove: only the bounded stage-one payload is sent
  before the success gate, stage-two pages only after success, exactly one
  visible root, the intake prompt without transcript interpolation, and
  failed/aborted/uncertain fixtures dispatch nothing.
- Release/bootstrap fixture tests install the plugin + runtime assets into a
  disposable HOME and exercise enable/disable/sync/doctor/uninstall with
  modified-file retention and no activation of live sessions. Detached
  worker spawns are test-injected (`SpawnWorker` in-process; the CLI honors
  `AGF_RETROSPECTIVE_NO_SPAWN=1`), so fixture tests never launch background
  processes.
