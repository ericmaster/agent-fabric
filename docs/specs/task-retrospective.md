---
title: "Spec: Task Retrospective"
type: spec
status: active
covers: internal/retrospective, cmd/agent-fabric (retrospective), hooks/retrospective, agents/learning-corrector.md
last_checked: 2026-10-15
---

# Task Retrospective

Opt-in, harness-neutral replay of one normalized task trace into an
evidence-linked report, then one visible correcting root for an eligible report.
Standard library only. No new dependency, framework, daemon, database, UI or MCP
server. The replacement loop is observe → ingest → deterministic signals/report
→ bounded semantic interpretation (only with candidates) → guarded dispatch.
Supervisor ledgers are not a retrospective database.

**Delivery status:** the implemented pipeline uses the single signal-first
analyzer for CLI/drain and production config. The old outcome/per-call/omission
engine is removed. Phase 1's `--signals-only` flag is retired with an explicit
migration error; `DetectSignals` remains the common provider-independent module,
not another evaluator. Controlled Phase 3 fixtures exercise actual file-task
before/after checks with protocol-mocked native roles. Their independent review,
supervisor QA and live/UI acceptance remain pending; this contract does not claim
live acceptance or deployment.
The model success gate, every-call classification, omission detection and
N>=20/>5% waste-rate routing are removed from the target contract, not retained
as an alternate engine. Requirement omission is deferred outside the minimal loop.

Canonical plan: `.artifacts-manager/plan-task-retrospective.md`.
Supervisor persistence is separate: `docs/specs/agent-fabric.md` (supervisor
ledgers are never a retrospective database).

## Files

- `internal/retrospective/config.go` — one config contract (defaults, thresholds, budgets).
- `internal/retrospective/trace.go` — trace schema v1, validation, normalization
  (ordering, dedup, task interval, steps, completeness).
- `internal/retrospective/redact.go` — local redaction before HTTP and before persisted evidence.
- `internal/retrospective/jev.go` — GLiDE client (legacy Go name; HTTP, retries, size-rejection splitting).
- `internal/retrospective/evaluate.go` — deterministic signal routing and compact interpretation (replacement).
- `internal/retrospective/report.go` — report assembly and atomic write.
- `internal/retrospective/store.go` — local state, locks, retention, settings entry.
- `internal/retrospective/dispatch.go` — session dispatch, canonical resolution, repair.
- `internal/retrospective/transport_boundary.go` — validated collection URLs and exact-origin credential authority.
- `internal/retrospective/native_learning.go` — actual v2 corrector/reviewer/QA execution and bound result evidence.
- `internal/retrospective/native_history.go` — strict v2 history/control/content adapter before backfill.
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
| `messages` | no | `[ {id, seq, role: user\|assistant, kind: message\|action\|final\|reasoning\|unsupported, text, hidden?, env?} ]`; durable native input uses only `role:context, kind:synthetic` |
| `calls` | no | `[ {id, seq, tool, status: success\|error\|failed\|aborted\|cancelled\|running ("" = running), exit_code?, input?, input_omitted?, result?, env?, prereq_fingerprint?, source_refs?} ]` |
| `requirements` | no | explicit requirements `[ {id, text} ]`; intent context only, no omission analysis |
| `child_refs` | no | linked descendant session ids that must resolve |
| `descendants` | no | full child traces (same schema, `session_id != root_session_id`); depth ≤ 16 |
| `gaps` | no | collection gaps `[ {kind, detail, session_id?, record_id?, start_seq?, end_seq?} ]`; optional inclusive range requires both nonnegative bounds, end >= start |
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
  retrospective compares digests for exact-repetition evidence and never
  invents one when absent.

Task interval resolution:

1. Host `task.id` present → interval `seq >= start_seq` (and `<= end_seq` when
   `end_seq > 0`), boundary source `host`.
2. Else the last `accepted_success` boundary → records with `seq > after_seq`,
   source `accepted-success`.
3. Else the whole session, source `session-start`.
4. More than one `task_start` boundary anchored inside the resolved interval is
   an ambiguous multi-task interval.

Completeness — each material condition makes the interval incomplete (never
repair-eligible): ambiguous multi-task interval; unresolved running
tool; missing linked child trace (`child_refs` without a matching descendant);
compacted-away evidence (gap `kind: "compacted"`); missing final assistant
response in the interval.

### Historical versus current collection gaps

Retention is coverage-critical: the common spool append seam deduplicates exact
observations BEFORE the 800-row cap. Equality requires owner session, kind,
producer record ID and both range bounds (including absent bounds). Anonymous
gaps are never deduplicated. Changed ranges/owners are distinct observations,
including in report IDs; descendant container namespaces cannot alias root IDs.
`duplicates` counts additional identical observations retained in a row, and is
included in report `deduplicated_gaps`. Counts do not assert global unique issues.
When ANY distinct row is discarded by count or byte limits, sticky spool
`gap_omissions` increments outside the row cap. Assembly emits a structured
`coverage_overflow` gap with `omitted` count, always unknown regardless of supplied
range or task boundary. It cannot be evicted by further gaps, restart or later
accepted boundary. Existing spool `overflows` also independently forces this
marker for known staged-record loss. Failure to fit fixed counters refuses the
write, never drops the marker. Load/write/assembly compact legacy over-cap rows
through this same seam; absent legacy loss counters cannot reconstruct past loss
and do not certify that no loss happened. No automatic counter clearing exists.
Compaction event eviction also increments `gap_omissions`. Counts saturate at the
platform maximum rather than wrap; negative persisted loss/count fields are
invalid. `omitted` counts discarded retained rows plus staged-record losses, not
globally distinct original issues or duplicate occurrences. Public native child
gap projections remain visible in assembled parent traces; normalization visits
an exact explicitly child-owned projection once via its present child original.
Root-owned claims in descendant containers never alias the root namespace.

The existing host `task` interval, else accepted-success boundary, else whole
session remains the only interval authority. A user follow-up, synthetic input,
session title, task-ID suffix, or current file contents does not invent a new
task/start/success boundary. Incomplete reports never advance accepted-success.

Gap scope is derived, not a trusted input field. `session_id` records the observed
owning session; `record_id` is a stable producer identity, never parsed from free
text. `start_seq/end_seq` describe the entire affected inclusive range in that
session's staged sequence. A lost record can use its known existing part sequence;
compaction conservatively affects the prefix `0..event.seq`, not just its marker.
Unknown/lost-start, transport, overflow and legacy free-text gaps stay unscoped.
No later source read manufactures historical provenance. Native descendants use
local sequence spaces: their gap ranges cannot be compared to the root interval.

Reports retain gap rows with `scope:historical|current|outside_interval|unknown`:
- `historical`: a known root-owned range ends at/before the resolved interval's
  exclusive start; retain the diagnostic but do not block this later interval.
- `current`: a known root-owned range intersects the interval; material incomplete.
- `outside_interval`: a known range starts after an explicit interval end; retain
  it without changing this interval's completeness.
- `unknown`: no complete range, foreign/descendant sequence space or conflicting
  scope/owner proofs for the same identity; conservative material incomplete,
  even when detail sounds historical.
An inferred last-visible-record seq is not an explicit task end: a gap for a
missing/private tail record still blocks that open-ended interval.

Stable report gap `id` is a digest of root/container-session/owner-session/kind,
producer record ID and range bounds, not of body/detail. Same identity is emitted once per report
(duplicate count retained); IDs stay equal across reports/windows for a distinct
issue projection to count once without mutating historical report bytes. Gaps
without record identity or range have no dedup identity: preserve each occurrence
rather than merge potentially distinct unknown issues. Detail/IDs/session fields
use the existing shared redactor before persistence. Scope does not make a gap
body trustworthy or grant source/write authority.

Producers scope only what they observe: part-specific capture/removal gaps use an
existing staged part's exact seq; private synthetic gaps likewise retain the part
identity without its body. Compaction controls retain native stable row IDs;
repeated native backfill of the same event preserves its first staged seq instead
of appending another marker while retained under the existing event cap. Re-seen
Identical retained observations keep the same gap identity; changed or lost range/owner evidence
never gains a guessed historical scope. Legacy events without IDs are not guessed equal.
Unknown legacy gaps are never retroactively assigned a range from a detail string.
Trace v1 legacy gaps remain valid and conservatively unknown. Report v2 adds
optional provenance/scope fields; prior strict report readers reject unfamiliar
fields rather than silently promote coverage. Old report bytes are never migrated.
Raw signals remain visible for incomplete intervals. Missing current source
provenance and all interpretation, citations, canonical/hash/WAL/review/QA guards
are unchanged; there is no partial-evidence semantic or write exemption.

Step definition: a step is a terminal tool attempt with paired input/result
(`input` present, `input_omitted` false, and `len(result) > 0`) or a non-final
visible assistant action message. User messages, the final assistant response,
native synthetic context, hidden reasoning, unsupported or binary messages, and unpaired or
`input_omitted` terminal calls are not steps. The legacy tail-step definition
remains readable for old reports, not a model success payload or routing gate.

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

## Deterministic detection (provider-independent)

Common entry point: `DetectSignals` → bounded detector data → `Analyze` →
`WriteReport`; public replay: `agf retrospective analyze --input trace.json
--output report.json`. Production `Config` embeds `SignalConfig` defaults and
accepts the documented flat detector keys plus conservative semantic thresholds.
Old `signals_only_phase1` previews remain inspectable, never dispatchable.
Whole redacted context records are retained under an aggregate 24,000-byte
compact JSON evidence-array bound (`evidence_limit_bytes`, only lowerable), with exact
omitted counts. Metadata/signal caps are separate; no byte/token equivalence.
Abstentions are scalar per-rule/reason counters; raw failures and cancellations
stay independent of semantic controls. No provider status/key is consulted.
Evidence for exact repetition records its observed prerequisite fingerprint;
failure status/exit/input and helper path/content hash remain evidence, not
avoidability labels. Private reasoning is deliberately excluded and counted,
not reclassified as a missing observable record. Binary/unsupported/secret-file
bodies are excluded with incomplete coverage; secret-file hashes are not copied
into the preview. Observed secret environment values are scrubbed wherever they
occur, not only in the environment map. The frozen fixture is
`fixtures/retrospective/signals/frozen.json`; tests do not resample live output.
Coverage separates `interval_records` from explicit `requirements`; their sum
is `collected_records`. Requirements remain intent only, never omission verdicts.
`record_dispositions` persists redacted stable IDs and retained/out-of-scope/
excluded/byte-cap reasons; omitted bodies are never persisted as truncated text.
Redaction covers quoted JSON secret-key assignments including escaped strings,
and observed secret env values in both literal and JSON-escaped input forms,
before packet construction and report persistence.

Normalize the resolved interval and descendants by stable `(seq,id)` order;
deduplicate call IDs before counting. Terminal calls exclude running/empty
status; compound commands remain one call. Rules use observed fields only.
Each signal stores `rule_id`, `rule_version: 1`, stable `signal_id` (SHA-256 of
rule/version and the first qualifying window's ordered evidence call IDs,
frozen at first emission), exact call IDs, observed values,
window positions, effective thresholds and coverage/abstention reasons. Detection
has no model probability or claim of avoidability. Equal observations must
produce identical signals with or without a provider. Apply exact comparisons
locally before redaction; persist/transmit only redacted evidence. Do not persist
unsalted input/content digests that may expose secrets; helper equality involving
secret-bearing content abstains. Detector identity uses call IDs, not secret bytes.

### Fixed rules and default windows

Windows count deduplicated terminal calls, not time or tokens. All rules operate
only inside one resolved task interval. Thresholds are positive integer config
values, recorded on every report; no generic rule DSL or heuristic NLP engine.

| rule_id | Exact default condition | Meaning, not conclusion |
|---|---|---|
| `terminal-failure` | Each terminal call with status `error`, `failed` or `aborted`, OR observed `exit_code != 0`; union counted once per call | Observed tool/command failure; cancellation counted separately, never inferred to be a nonzero exit |
| `exact-repeat` | At least 3 calls in a rolling window of 20 terminal calls with byte-identical nonempty tool and input, `input_omitted=false`, and the same nonempty observed `prereq_fingerprint` | Repetition under collector-attested equal prerequisites, NOT redundancy/avoidability |
| `helper-recreate` | At least 3 successful explicit `write` calls in 20 terminal calls, same resolved ephemeral path AND exact observed content SHA-256 | Exact repeated helper creation only, not semantic similarity |
| `search-burst` | At least 5 explicit `grep`/`glob` calls in 10 terminal calls | Search sequence needing context; legitimate research may justify it |
| `broad-search` | At least 3 explicit `grep`/`glob` calls in 10 terminal calls whose explicit path equals the declared execution root | Observed broad scope, not excessive returned files or automatically a problem |
| `late-context` | In first 10 terminal calls, at least 5 recognized search calls precede the first successful observed instruction read (or none observed by call 10); emit only once at call 10 | Early acquisition sequence indicator, NOT proof that context was absent or needed |

For repetition/helper rules emit one signal per matching key per task, at its
first qualifying window; later matches append call IDs and count to that signal.
For search rules emit one per rule per task at the first qualifying window,
retaining the window's exact call IDs. Failure signals are one per call. Report
total matching windows/counts separately; overlapping windows never inflate
unique-call totals. At fewer than 10 calls `late-context` abstains, not a negative.

Schema support is deliberately narrow and versioned with the rules:
- Generic terminal status/nonzero exit uses trace v1 fields, even for unknown
  tools. Exact repetition uses trace v1 opaque tool/input/fingerprint fields;
  absent or changed fingerprints cannot assert unchanged prerequisites.
- Search recognizes tool names exactly `grep` and `glob`, with input a JSON
  object containing string `pattern` and optional string `path`; optional grep
  `include` is context only. Count recognized calls even without path; omitted
  path cannot count as broad. Reject malformed/type-incompatible shapes for
  these metrics and record abstention. No shell grep/find/ripgrep parsing,
  returned-result breadth, inferred current directory or unknown-tool aliases.
- Context acquisition recognizes successful `read` calls with JSON string
  `filePath` or `path` and a call-linked at-use `source_refs` path/hash for an
  instruction source as defined below. Conflicting path keys abstain. A source
  known elsewhere but not linked to this call cannot establish its sequence.
- Helper creation recognizes successful tool `write` with JSON string
  `filePath` and `content`, absolute cleaned path under `/tmp/` or `/var/tmp/`
  (including `/tmp/opencode/`), suffix `.py`, `.sh`, `.js` or `.ts`. Hash the
  exact observed content locally and retain path/hash with the signal only when
  safe under redaction. Path alone or hash alone is insufficient;
  absent content, relative paths, shell heredocs, patches and arbitrary helper
  aliases abstain. Same content at different paths does not match. This is an
  intentionally limited proxy; no claims about usefulness, similarity or all
  ephemeral helpers. No capture extension is necessary to synthesize evidence.

Expected red tests, no-match searches, polling and legitimate research remain
visible observations/negative interpretation controls, not blanket detector
exclusions. Missing schemas/provenance yield explicit per-rule abstentions;
no fabricated duration, token, cost or result-size estimates.

## Bounded GLiDE interpretation and routing

Every signal is visible independently of interpretation. Zero signals means
`interpretation.status=skipped_no_candidates`, zero provider requests. Otherwise
interpret candidate groups, not each call and not task success. GLiDE chooses
`actionable|justified|uncertain`; auxiliary choices identify
`missing-context|instruction|tool|other|uncertain` causes. Context/tool bottlenecks
are semantic judgments, never deterministic labels. Choices refer only to
supplied signal/evidence IDs. For each signal, a support Choice selects one exact
supplied evidence call ID or `insufficient-evidence`; actionability/cause
questions bind to the same ordered evidence set. Require a valid observed support
ID for actionable routing, not a fabricated prose reference. The support Choice
is a locator, not independent causal proof. Corrector investigation supplies
any further causal explanation with citations; GLiDE's Choice API is not an
invented free-prose citation interface. Instruction defects, ignored instructions,
environment recovery and implementation bugs remain distinct downstream.

Each candidate packet contains its redacted evidence, all in-interval native synthetic context, all task user intent
messages/explicit requirements, observed terminal outcome, final response if
present, source provenance references, plus the 2 preceding and 2 following
terminal calls around each signal evidence call (deduplicated, ordered). Include
visible assistant action messages between those calls. Report exact included
and omitted record IDs/counts and gap reasons. Records outside this documented
neighborhood are explicitly `out_of_context_scope`, not a collection gap or
claim of full model coverage. Omission of any required neighborhood, intent,
signal or source evidence is incomplete, not an innocuous scope exclusion.
No whole-task per-call questions,
omission questions, model success request or paginated full-trace classification.
Wire state holds one packet under `state.evidence[signal_id]`, referenced by
three ID-bound questions (`action:`, `cause:`, `support:`). Evidence is not
triplicated per question. A packet digest covers only this redacted packet;
dispatch rechecks its exact evidence IDs/digest and support call membership.
Missing intent, material collection gaps, unsafe redaction or an indivisible
oversized packet retains local signals but skips semantic actionability;
safe partial evidence may be displayed locally as advisory, never a repair basis.

Persist at most 100 signals in first-evidence `(seq,id,rule_id)` order, each with
at most 20 evidence call IDs; retain full scalar totals and omitted counts. Local
report context uses the same bounded selection. Any signal/evidence omission
sets `evidence_complete=false` and forbids automatic correction for that report.
Interpret at most 10 retained signals per task (same order); each gets one
self-contained packet, never fragments that imply unseen context. Skipped
signals stay visible with reasons. Aggregate byte limits can reduce delivery,
not silently trim. No unrelated full trace is sent to fill a request.

Conservative actionable routing requires selected `actionable` probability>=.90
and confidence>=.80, valid cited IDs, complete delivered packet and safe
redaction. These are uncalibrated semantic-actionability defaults, not detector
confidence, proof of cause, task success or write authority. Justified, uncertain,
low-confidence and unavailable interpretations remain in the report. Remove
N>=20/>5% waste-rate routing entirely; one observed candidate can be interpreted.
No avoidable-call denominator or modeled waste statistic remains.

Task terminal outcome is a separate observable field: copy `terminal_status`
and whether supplied; absent/default `ok` means no recorded error, NOT proven
success. Host error/abort never becomes success through interpretation/repair.
Failed/aborted tasks with complete relevant evidence may produce actionable
advisories; automatic writes additionally require the explicit safeguards below.
Missing final/child/boundary evidence still blocks writes, not local diagnostics.
An observed error/abort in a fully collected descendant is likewise not a
synthetic `partial_stream` gap. Keep genuine child gaps/missing references,
call statuses and source-session IDs; do not confuse unsuccessful work with
missing collection or reinterpret it as successful work.

### Provider and transport safeguards

Fastino GLiDE is pinned to `fastino/GLiDE`, `POST /v1/systemone` at default
`https://api.fastino.ai`; host `FASTINO_API_KEY` is sent only as `X-API-Key`,
never logged or persisted. Missing key means unavailable, not missing signals.
Jev remains rejected pending verified contract; legacy Go names do not imply
another engine. Authoritative API: [System One](https://docs.fastino.ai/inference/systemone),
[Choice](https://docs.fastino.ai/concepts/glide-choice),
[OpenAPI](https://docs.fastino.ai/openapi.json) (checked 2026-10-13).

Wire `{model,state,questions}` uses ID-keyed Choice questions with instructions
and 2–255 named criteria; evidence is folded into `state.evidence[id]`. Require
exact question/choice IDs, response model exactly `glide` or `glide_v2`, Choice type, finite confidence and
complete probability distribution in [0,1], sum within .00001 of 1, chosen
probability maximal. Reject duplicate JSON keys, depth>128, unknown response
fields except documented ignored usage/token_usage, forged IDs and malformed
choices. Invalid/missing evidence abstains, never invents citations/verdicts.

The exact `glide_v2` compatibility value is observed live evidence, not an
officially listed alias: the 2026-10-15 authenticated pinned request returned it
(sanitized evidence: `.artifacts-manager/agf1-glide-model-discriminator-evidence/`).
OpenAPI declares an engine-name string (example `glide`); the guide still says
to expect `glide`. No arbitrary string, prefix match or future version is accepted.
Missing/null/wrong-type/unrecognized models fail closed; unrecognized string
models surface `unknown_provider_model` without echoing the value/body.
The model marker is routing compatibility, not proof of response authenticity.
Report `model.model` remains the pinned requested model `fastino/GLiDE`, not the
observed response engine; no engine observation field is added by this fix.

Preserve actual wire limits: 24,000 UTF-8 bytes/request and 16,000 bytes folded
state+longest keyed question, measured by the actual wire encoder before send;
these are conservative byte bounds, NOT token estimates (provider state+question
limit 40,000 tokens). Limits may only be lowered. Oversize/413 becomes visible
uninterpreted evidence, not omission disguised as complete interpretation.
Default request budget is 30 HTTP attempts per task (10 packets x 3 attempts),
timeout 30s, max attempts 3, backoff 300ms doubling to 4s, Retry-After capped at
30s; all count against budget. Retry transport/5xx/425/429/503 and malformed
responses within budget; 401/403/404 permanent; all redirects refused, including
credential replay. Encoding failures stop locally. Responses>4MiB are rejected,
not truncated. Provider faults/budget exhaustion persist signals with exact
unavailable/uncertain reasons and no automatic repair. No forced live decision.

## Report schema v2 and compatibility

Write reports atomically, mode 0600, for every structurally valid trace even if
failed/aborted/incomplete or provider unavailable. Retain:
- `schema_version: 2`, UTC generated time, detector/rubric versions and effective
  config; trace identities, interval/source revision/input digest, source hashes,
  descendant coverage, dedup counters and redacted gaps.
- `terminal_outcome` with observed value/presence and independent completeness.
- `signals[]` as above, counts per rule/unique calls/raw failures/cancellations,
  rule abstentions and exact omitted signal/evidence/context counts.
- `interpretation` status, per-signal typed decisions, evidence IDs, probabilities,
  confidence, model/rubric/HTTP metadata and reasons for every skipped candidate.
- `coverage` separating collected/detected/retained/sent evidence, dispositions,
  source/record digests and `evidence_complete`; redaction counters and safety.
- `routing` candidate/actionable flags, thresholds, explicit advisory vs repair
  eligibility/reasons; no `stage_one`, `stage_two` or waste-rate gate.
- `dispatch` eligibility never alone authorizes writes; downstream verified
  findings and independent native review/QA remain mandatory.

`status` describes collected evidence completeness independently of
`interpretation.status`. A complete signal report can truthfully record an
unavailable/incomplete interpretation and an error/aborted source outcome.
Redaction counters are `redaction_substitutions`; the safety/omission verdict
is `coverage.evidence_complete` with record dispositions and reasons.
`Analyze` always emits `repair_safe:false`: `dispatch.eligible` authorizes only
an independently guarded investigation. Before native application, the exact
candidate's completed independent reviewer verdict, collected source version,
failed behavior/recovery and installed QA validation capability permit the
local application copy to set `repair_safe`. A successful semantic judgment
alone cannot do this. Existing causal/canonical/hash/WAL checks still run at
the write sink, and postimage QA failure restores only the matching repair.

Trace/envelope schema v1 remains supported; no invented input fields. Legacy v1
reports remain inspectable, labelled legacy, but cannot dispatch new repairs;
re-analyze original evidence into v2, never translate old success/trigger bits
into write authority. Existing journals/receipts keep their original identity;
resume validates recorded hashes/provenance and does not replay old evaluation
or duplicate native roots. Remove obsolete success/rate/tail config keys with
explicit migration errors rather than silently retaining two evaluators.
For v1, only an exact already-persisted native pending WAL finding with the same
report hash/incident, root, independent review and preimage can resume; existing
journal/reviewer/hash/mode/QA checks apply. Unjournalled/queued legacy reports
stay read-only and require explicit re-analysis, never blind redispatch.

## CLI

### Reusable lessons (learning consolidation phase 2)

AGF owns a private, versioned `lessons.json` in the existing retrospective Store.
Offline signed ingestion and explicit native generation share admission/recall.
GLiDE selection, runtime profiles, permissions, observation and existing repair
dispatch remain unchanged. No legacy state is read.

`agf retrospective lessons apply --state-dir ABS --trust ABS --input ABS`
accepts one strict JSON envelope: `candidate`, base64 `proposal_signature`, and
`acceptance` (`candidate_sha256`, `reviewer`, `approved`, base64 `signature`).
The candidate has `schema_version:1`, `operation:add|refine|retire|noop`, stable
`lesson_id`, `expected_revision`, `project` (absolute execution root), `scope`,
`version` (exact compatibility identity), `consumer:executor|corrector`, advisory
`advice`, `producer`, `origin_session`, and `evidence` (`path`, `sha256`,
nonempty `record_ids`). Add requires revision zero; refine/retire require the
current positive revision and exact applicability identity. Noop has no lesson
ID, revision or advice. Retire has no advice. Refine cannot revive a retired ID.
Candidate SHA256 over Go's canonical typed JSON is also the validated delta ID.

Trust is explicit operator authority, never candidate/transcript input: `--trust`
is a strict JSON document with schema_version 1 and `identities` mapping IDs to
`role:producer|reviewer`, base64 Ed25519 `public_key`, and exact `project`.
Independent acceptance requires distinct identities AND distinct public keys;
both must belong to the candidate project. Producer signs
`agf-lesson-proposal:v1\n` + candidate digest. Reviewer signs
`agf-lesson-acceptance:v1\n` + candidate digest + `\n` + reviewer ID + `\ntrue`.
An approved boolean or reviewer name alone is never authority. This authenticates
the operator-enrolled reviewer key, not the reviewer's reasoning or a live model.
Protect trust enrollment and private keys outside model-controlled input roots.
This offline seam does not attest native execution. Native generation below uses
actual host-verified role execution instead, never signatures on arbitrary prose.

Before accepting any operation, read the exact bounded regular non-symlink
evidence file, verify SHA256, strictly decode trace v1, normalize completeness,
check exact execution root/source revision/origin session, verify cited observed
record IDs in the task interval, and run existing deterministic coverage/redaction
checks without a provider. Unsafe/redacted/secret-file/incomplete evidence is
rejected, not silently repaired. Candidate/trust metadata also passes existing
redaction and prompt-injection guards. Learning origins and synthetic evidence
cannot become new human task provenance. Store-origin suppression is checked.
Privacy checks visit every raw message/call, including duplicate IDs and all
descendants, before normalization can discard records; raw bytes are retained
only when every occurrence is safe. Material `input_omitted` in the accepted
task interval rejects the evidence even when the call is uncited or outside a
signal neighborhood. Only an explicit host task end bounds that interval's tail;
historical/outside-host-interval omitted input does not block a later task.

One named Store lock serializes read/compare/write mutation. One private atomic
snapshot contains signed delta records and exact validated trace bytes; replay
derives current lessons and dedup receipts without temporary input files.
Caps: 256 stable lesson IDs, 1024 delta receipts, 64 KiB per trace, 16 MiB snapshot;
full storage rejects writes rather than evicting dedup/retirement authority.
Same delta is idempotent; same applicability/advice/evidence under a new signed
proposal is a duplicate, never a revision/win promotion. Conflicts, unknown
schemas, bad signatures, corruption, missing evidence and failed writes fail
closed and preserve existing bytes. Only missing state means empty.

`agf retrospective lessons read --state-dir ABS --trust ABS --project ABS
--scope NAME --version NAME --consumer executor|corrector [--limit N]` returns
only active exact-applicability lessons, sorted by stable ID, at most 20 (default
10). Output marks `advisory:true`; advice cannot grant task/DoD/permission authority.
`... lessons status --state-dir ABS --trust ABS` returns active/retired/delta counts.
Both validate persisted signed/native authority, evidence bytes and state structure; neither
creates state, reads ambient config, invokes a provider, nor installs anything.
Exit 1 denotes invalid inputs/state or a refused mutation; exit 0 returns JSON.

#### Native lesson generation and contrasts (phase 4)

`lessons generate --config ABS --trust ABS --input ABS` is explicit opt-in, not
automatic drain generation. The existing global enabled switch and scoped disable
marker apply. Config supplies the existing `message` transport, dispatch server,
explicit configured dispatch model, canonical roots, state directory and installed
independent reviewer (not learning-corrector/qa-runner). There are no CLI
model/provider/permission overrides. Trust remains required for the mixed store's
offline records; native authority does not enroll keys or sign model output.

Input is an unsigned LessonCandidate identity/evidence template: operation noop,
empty producer/lesson ID/advice, revision zero, no reflection. Operator-selected
project/scope/version/consumer/origin and evidence path/hash cannot drift. Its
record IDs are a bounded allowlist. Complete safe trace validation precedes network;
no deterministic signal means no native request. Existing origin suppression,
exposure suppression, configured binding exclusions and known descendants gate
generation before creating roots and again before admission. Read/status remain
deterministic and require no provider.

Reuse the native SessionClient/stage/completion guards: parentless learning-origin
roots, read-back directory, at-most-once attempted create/prompt reconciliation,
bounded wait and effective actual role/model/error/completion history. The
learning-corrector returns one strict candidate JSON (add or noop); Go binds its
producer to the actual root. A different root executing the configured reviewer
must return strict `{approved,candidate_sha256,reason}` bound to the canonical
candidate digest. Author approval, HTTP delivery, idle, wrong role/model, provider
failure, incomplete history or rejected/mismatched verdict never activate advice.
Typed JSON rejects duplicate/unknown fields, including self-declared confidence.
Finals are capped at 64 KiB. Unsafe finals are refused without retaining their
secret/injected bytes. No source repair, QA role, skill/hook/policy write, global
promotion, provider change or external-effect replay occurs in this workflow.

Candidate `reflection` separates `facts` (record ID + SHA256), `change`,
`contrasts` (before/after citations), `outcome` (helped, hurt, no observed effect,
confounded) and explicit `limits`. Citation SHA256 covers Go's JSON of the
normalized observed Record; the generation packet supplies exact records/digests,
not a request to invent or recompute hashes. Every citation must resolve inside
the accepted task interval and candidate allowlist. Active native advice needs
at least two nonoverlapping natural contrasts of the same input substitution:
adjacent terminal paired calls, same tool, nonempty unchanged prerequisite
fingerprint and unchanged observed source refs. Helped requires error→nonerror;
hurt the reverse; no observed effect equal error states. These are status
observations, not task acceptance or causal benefit. Semantic confounders and
advice relevance still require independent review. Single contrast, selected
winner, multiple observed changes or confidence cannot prove benefit;
confounded/uncertain conclusions must abstain (noop, never active).

The private Store receipt retains attempted stage identities, configured model,
typed results/rejection and completed-review authority. Exact safe trace bytes,
including negative/discarded observations, use existing durable `preimages/`
retention (`<incident>/lesson-evidence`), separate from prunable reports; retaining
these bytes never promotes code. The lesson snapshot references the host-owned
completed receipt plus exact evidence bytes. Reads revalidate this authority,
candidate/verdict digest/independence and evidence; missing/corrupt authority fails
closed. Public signed apply cannot supply a native receipt or bypass signatures.
Pending stages reuse the recorded profile and roots without blind retries.
Native generation is add/noop only; signed refine/retire remain available.
Existing v1 signed records without reflection retain their authenticated offline
contract; supplied reflections are checked, not silently grandfathered.

`TestLessonGenerationCLI` runs real CLI processes/transport/admission/recall with
protocol-mocked native roles. It proves wiring and negative gates, not live role
execution, independent provider review or phase6 native two-task E2E acceptance.

#### Contextual delivery (phase 3)

The native OpenCode v2 plugin optionally uses the verified `session.hook("context")`
API, appending one nonhuman advisory SystemPart, never changing human prompts,
durable transcript, model/options/permissions, or `load-task` JSON. Opt-in requires
the existing global `retrospective.enabled` AND an explicit `AGF_LESSONS_CONTEXT`
absolute binding file. Unset binding, disabled global/scoped switch, absent session
binding, or empty memory leaves ordinary tasks unchanged. Other harnesses use the
explicit `lessons read` command above: keep its output separate from human intent
and task authority; no automatic integration is promised.

Binding v1 is strict JSON: `schema_version:1`, `state_dir`, `trust` (absolute),
`sessions` mapping exact native session IDs to `{project,scope,version,consumer}`,
and `excluded_sessions` (array of monitor/learning root IDs). Native delivery is
executor-only; corrector recall remains explicit. Operator controls binding and
trust, not transcript text; no prompt parsing or inferred applicability. Monitor
roots must be explicitly excluded; learning/corrector roots additionally retain
the existing learning-origin marker/store suppression. Both apply to descendants.
When `AGF_LESSONS_CONTEXT` is explicitly configured, `lessons apply` loads that
same strict operator binding and requires its `state_dir` to match the admission
store. The shared origin-admission check rejects excluded IDs and their known
spool ancestors, including every evidence descendant, before any lesson delta
(even an idempotent retry). This applies independently of delivery enablement,
session query entries or prior context calls/receipts. Malformed, unreadable or
wrong-store bindings fail closed; exclusions never come from proposal input.
Without a configured binding, existing explicit offline admission is unchanged.

`lessons context --binding ABS [--ack SHA256]` receives a strict bounded stdin
object `{session_id,project,sessions}`; sessions maps the complete current native
ancestry to `{parent_id,title,directory}`. Unknown/cyclic/overlong ancestry is an
explicit error, not permission to retrieve. All directories must equal project.
Store-origin rows, learning marker titles, excluded IDs, and scoped opt-out prevent
delivery. All selection, suppression, formatting and receipt validation live in
Go. Output is `{block,sha256,lessons}` with at most three `{id,revision}` references
and an 8 KiB block, selected by the existing exact project/scope/version/consumer
reader. It never calls a model. Missing state is empty; corrupt state/trust is a
diagnostic, preserved byte-for-byte, not an empty success.

The hook fetches current native ancestry (max 32), with bounded time/output for
CLI calls, appends the block, then acknowledges its digest. Ack reselects against
current state and refuses mismatches; a failure removes the appended part before
returning and reports a private diagnostic without failing ordinary generation.
Ack atomically writes one bounded latest `lesson-deliveries/<session>.json` receipt
with session, exact applicability, IDs/revisions and block digest. The receipt means
the hook appended context, not successful model consumption or causal benefit.
It is not a native/provider acceptance claim. Context is request-local, so capture
never sees it as human/synthetic provenance; existing synthetic-evidence rejection
continues to prevent advisory input from originating independent lessons.
Additionally, ingestion rejects origins with delivery receipts and their known
descendants: conservative task-level abstention prevents recycling invisible
request-local advice as fresh experience. Own-origin lessons are not delivered.

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

`internal/retrospective/config.go` remains the single implementation owner of
defaults. The deterministic-rule section above owns their normative values;
do not duplicate them in adapters. Replacement keys: `repeat_min_calls`,
`repeat_window_calls`, `helper_min_calls`, `helper_window_calls`,
`search_min_calls`, `search_window_calls`, `broad_search_min_calls`,
`early_context_window_calls`, `early_context_search_min_calls`, `signal_cap`,
`signal_evidence_cap`, `interpretation_signal_cap`, `context_neighbor_calls`.
Broad-search shares `search_window_calls`; early-context window is also its
minimum terminal-call sample size. Counts must be positive integers, minimum
counts <= their windows; caps/context neighbors may only be lowered from the
documented defaults. Rule IDs/versions and supported tool schemas are shipped
constants, not user-defined executable rules.

Keep existing endpoint/evaluator, request byte limits, timeout, maximum attempts,
backoff/Retry-After and additive `redact_patterns` keys. Set `request_budget` to
the bounded interpretation budget above. Replace success/rate controls with
`minimum_actionability_probability` and `minimum_actionability_confidence`.
Thresholds may be raised, not lowered, for autonomous routing. Pin the shipped
rubric `task-retrospective-signals-1`; the provider model remains fixed.
Duration fields (`http_timeout`, `backoff_base`, `backoff_max`) are
unit-suffixed strings. A bare number is rejected; it is not interpreted as
nanoseconds.
Dispatch reads this same struct and the dispatch keys below.
`evaluator` defaults to `glide`;
`jev` is rejected pending verified contract. The pinned evaluator model is not a
config key; a `model` key remains unknown.
Explicit null config values are rejected; omit optional keys to retain defaults.
Retry-After caps may only be lowered from 30s. Provider failures retain typed
diagnostic reasons (size rejection, budget, malformed response, rate limit,
permanent rejection, unavailable/timeout), never arbitrary provider error bodies.
Also retired: `min_fragment_bytes` (there is no full-trace fragmentation).
`--signals-only` returns an explicit retirement error before producing output;
omit it to run the single pipeline. Legacy report files themselves are unchanged.

## Testing contract

### Controlled Phase 3 evidence (not live acceptance)

The frozen positive/negative observations must also traverse public CLI analyze,
preserving exact rules/call IDs, counts, abstentions, coverage and advisory routing.
Do not add missing intent to frozen inputs to force semantic eligibility.
Controlled native repair tests may inject predetermined interpretations ONLY
through the existing test ChoiceClient. Start with Analyze's original failed
task report (`repair_safe:false`, no injected findings); use public Dispatch and
the existing native protocol mock for corrector/reviewer/QA, not direct apply.

A disposable catalog lookup task reads an owning Markdown instruction, then
loads its declared JSON catalog and checks a known SKU/price. Its stale path
must fail on the original instruction, while a direct read proves the canonical
catalog exists. The exact same task succeeds only after the Go-owned minimal
repair; unknown SKU/wrong price controls still fail. A read-only protocol-mocked
reviewer checks the exact candidate/source/digest and a separately identified
protocol-mocked QA checks actual postimage and task results. Their typed results
are synthetic protocol evidence, NOT actual independent model execution.
Do not grant native roles forbidden tools: this fixture requires only file reads,
JSON interpretation and equality checks; the Go test harness executes the checks.
Retain before/after output, source hashes, proposal/digest and role/root receipts.
Reject review/digest failures, preserve dirty source, and roll back only a matching
postimage on QA rejection. Reuse existing protected-target/provenance/cause,
incomplete-evidence, WAL/restart, origin/dedup and concurrent-edit regressions.

Phase 3 fixture/test readiness never closes live native/UI acceptance. Actual
fresh independent code review and supervisor-scheduled QA remain mandatory;
live corrector/reviewer/QA roots, reconnect desktop/mobile evidence, and natural
GLiDE end-to-end learning are UNMET until separately demonstrated.

- Unit tests in `internal/retrospective` and `cmd/agent-fabric` cover the
  deterministic signals/compact interpretation/redaction contracts with recorded mock
  HTTP (`httptest` servers) and synthetic traces only.
- Fixtures are synthetic and disposable; they may only contain synthetic
  secrets (`CANARY-*` strings). Tests prove canaries never reach HTTP bodies,
  persisted reports, or logs.
- `go test ./internal/retrospective/... ./cmd/agent-fabric/...` plus the repo
  regression `go test ./...`, `go build ./cmd/agent-fabric`, and
  `python3 -m unittest discover -s tests -p 'test_supervisor_ledger.py'` stay green.
- Phase 2 session tests use `httptest` only. They never dial a real OpenCode server.

## Phase 2 — dispatch and verified text repair

The headings below identify existing dispatch/observation subsystems, not the
remaining implementation phases; the canonical plan owns phase ordering.
Detection metrics are not repair safety decisions. A report with incomplete
collection, omitted evidence/context, unsafe redaction, uncertain/unavailable
interpretation or unsupported causal claims cannot authorize writes. Keep
signals/advisories inspectable regardless of dispatch eligibility.

An unsuccessful source task does not alone prohibit diagnosis or repair, nor
waive a single write guard: require complete causal evidence of the exact
instruction version and demonstrated failed behavior plus supported recovery,
explicit `repair_safe` with reasons, and a reproducible bounded validation task
that can show the proposed correction works. If recovery/validation cannot be
established, advisory only. Source outcome stays error/aborted even after a
successful separate repair task. No deterministic hit alone can set `repair_safe`.

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

`dispatch` validates v2 report schema, conservative semantic actionable routing,
`dispatch.eligible`, explicit `repair_safe` before any write, complete coverage
(including zero omitted/undelivered required evidence),
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

Native setup requires explicit absolute `AGF_BIN` before registering capture hooks;
there is no native CWD/HOME/PATH fallback (the v1 replay adapter retains its legacy
resolver). The executable and immediate containing directory must be owned by the
current uid, non-group/world-writable, with no symlink components; the file must
be executable. Use a versioned owned release directory, not the protected-main
symlink. Ancestor directory write policy is not a generic filesystem sandbox.
One bounded 5s, 4KiB-output preflight runs exactly `retrospective ingest --harness
opencode --help` with ignored stdin. Compatibility means exit 1 and exactly the
current Go parser's `agent-fabric: flag: help requested` stderr, empty stdout:
the old CLI rejects the ingest arguments instead. This tests command parsing,
not a filename/version string, and never invokes ingest/evaluation/state writes.
Missing binding spawns no child; incompatible binding only the preflight. Refusal
returns an inert disposable instance with one private diagnostic, no capture hooks.
Each ingest rechecks the same path's identity/mode/content hash against preflight;
changed binding permanently declines once, without disabling valid transient retries.
This is deployment stability checking, not isolation from a malicious same-uid writer
racing the final check/spawn; deployment must keep release bytes immutable in use.
Diagnostics include bounded observer pid, timestamp, explicit CLI/resolution source,
and child pid when available. A declared observer origin is emitted only for clean
HTTP(S) origins without userinfo/query/fragment; it is not verified producer identity.
Preflight output/errors are never copied to diagnostics, only fixed reason codes.
No process-global environment mutation/pin is used; reload gets fresh instance state.
Global installation does not prove all-session coverage: only the activated and
verified origin (initial rollout: 4097) may be claimed.

Native support is an explicit allowlist: `2.0.24`, `2.0.27` and `2.0.30`, never a semver
range. Both require session prompt/get, tool execute.after and event subscription
capabilities before staging. Unsupported versions/missing capabilities fail closed
with an actionable error and full runbook locator. The typing dependency remains
`@opencode/plugin@2.0.24`; no dependency upgrade is required for the unchanged hooks.
Tag `v2.0.27` is commit `1f4d07a4a519185955ae8ab2ea3607805509e386`.
Exact-tag plugin Promise/effect session/tool/event contracts and session/event
schemas retain the consumed 2.0.24 wire shapes. The 2.0.27 history idle control adds
optional `error`; the history adapter accepts that field without promoting it to
a message, exposing its body, or deriving task success. Outcome still exclusively
maps failed/interrupted controls; strict unknown/type-incompatible fields remain
rejected. Session create role/model/permissions/location, prompt text, history
content and all independent execution/write guards remain unchanged.
Context7 CLI documentation is useful orientation but unversioned (`dev`); exact
tag sources, literal fixtures and isolated runtime evidence own version claims.
Evidence and live-versus-mock limitations:
`.artifacts-manager/agf1-native-2027-handoff.md`.

The original native contract baseline is OpenCode tag `v2.0.24`, commit
`e7a34f09bfd9134dfade5a8ddb843f7030bc9a69`:
`packages/plugin/src/README.md`, `packages/core/src/plugin/module.ts`,
`packages/schema/src/session-event.ts`, and
`packages/protocol/src/groups/session.ts` in
https://github.com/anomalyco/opencode/tree/v2.0.24 . Compare only the explicit
2.0.27 tag above and 2.0.30 below, not master/dev, for additional supported versions.
Tag `v2.0.30` is commit `f16ef9ad8878279ea2be3cfab7baeed97aef61aa`.
Its plugin README/loader, session-event/session-message schemas and session API
are byte-identical to 2.0.27. The CLI adds Claude skill-root disable handling,
not a consumed wire change. Keep the 2.0.24 typing pin. A version update requires
fresh exact-tag comparison, installed-byte pins, disposable default-loader and
public session/history/event checks without providers, malformed/control and
legacy regressions; neither package metadata nor health alone grants support.
Installed version is not a running deleted inode's API version. Live activation
must separately verify its actual producer. Compatibility evidence and limits:
`.artifacts-manager/agf1-current-runtime-compat-handoff.md`.
The 2.0.30 literal public history fixture is an actual HTTP projection of an
imported synthetic transcript (not new model/tool execution). Its role/model
controls and queued prompt events come from actual disposable public endpoints;
replay flush triggers are labelled synthetic. Raw public responses and the real
default-loader-to-Go-ingest spool are retained separately from protocol mocks.

Native history also validates the exact-tag `synthetic` union member: base
identity/time/optional metadata, required string `text`, optional string
`description`, and no other fields. This is durable model-facing session input,
not ignorable metadata. Preserve public text as a message with the exact pair
`role:context, kind:synthetic`, normalized kind `synthetic_context`; it is neither
human intent nor assistant authorship. Native part type `synthetic` carries its
stable message ID, owner session and existing staged sequence. It is distinct
from the legacy v1 `synthetic:true` ignored-part flag. It does not create a user
task boundary, supply assistant→user ownership, select a final, count as a step,
record a tool attempt or attest at-use source provenance. Existing session parent
relations and task intervals are preserved. Missing human intent still abstains.
Description is display-only and metadata is an opaque optional JSON object;
neither is copied to messages, diagnostics, evidence or privileged routing.
Present metadata must be a non-null object (any string-keyed JSON values);
scalar/array/null values, duplicate keys and nesting beyond 128 fail closed.
This base-metadata validation applies to every native history row, not just input.
Missing/null/wrong-type text, null/wrong-type description, oversized
text/description or incompatible fields fail closed. Synthetic-only history
still lacks assistant evidence. Source: exact `v2.0.30` commit above,
`packages/schema/src/session-message.ts` (`Synthetic`, `Info`) and
`packages/protocol/src/groups/message.ts` (`PublicSessionMessage`). A bounded
existing managed-session structural capture observed `type:synthetic` with
`id,time.created,text`; no transcript body was retained.

Exact production/consumption: session API `session.synthetic` durably admits input
and schedules execution; `core/src/session/projector.ts` projects admitted input
to the `synthetic` history row with text and ID; the exact-tag
`core/src/session/runner/to-llm-message.ts` passes its text to the LLM as user-role
content. Retrospective preserves the native source distinction rather than
claiming human authorship from that provider-facing role.

Every in-interval synthetic-context record is required bounded task context in
the local evidence and every signal candidate packet, even outside the tool
neighborhood. Apply the same local redaction before spool/report/HTTP, whole-record
evidence caps, stable-ID deduplication and interval exclusion as other records.
Omitted/unsafe context makes coverage incomplete and skips semantic routing;
there is no context-only completeness exemption. Evidence records additionally
carry `source_kind:opencode.synthetic` (not a source-file hash or trust authority).
Shared `Redactor.Record` applies the existing message text/environment rails to
`synthetic_context`, including direct Analyze, detector-only replay, CLI input and
descendant normalization; native spool redaction is not a substitute. Observed
secret environment values (literal and JSON-escaped) are scrubbed from context
before whole-record UTF-8/encoded-byte budget decisions, report persistence or
candidate HTTP. Unknown record kinds have no transmittable body at this shared
redaction boundary; private/hidden/binary/unsupported records remain excluded.

A native synthetic part marked `reasoning:true` is private regardless of its
public-looking part type or context role. Its body is cleared before spool
persistence and withheld before trace message construction, with a fixed material
`partial_stream` gap; prior persisted private parts are likewise withheld on read.
The reasoning flag is monotonic under upsert: a late privacy flag clears retained
content, and a later false/unset flag or part-type relabel cannot republish it.
This applies to descendant spools too. Direct trace hidden/binary context is
excluded with incomplete coverage; a context/reasoning kind mismatch fails
normalization, never reclassifies private text as public context. Public synthetic
input without privacy flags remains material retained context, not a blanket gap.
Trace v1 remains readable for legacy user/assistant messages; only the exact
new role/kind pair is accepted. Other role/kind combinations fail normalization.
Report v2 gains this optional provenance field only on native synthetic context:
old strict report decoders reject it rather than silently consuming an unknown
context kind; old trace normalizers reject the new role. New pipeline tests must
cover full public reconciliation→Analyze→packet and strict report round-trip,
legacy message compatibility, intervals, caps and deduplication. No old report is
rewritten or promoted to new authority.

The remaining exact-tag union members `system`, `skill`, and `shell` are audited
but **not** synthetic exclusions: they carry frozen model-facing context, skill
activation text, and explicit shell execution respectively. They remain material
unsupported evidence (fixed public type diagnostics, no raw payload/unknown type
echo), never silently ignored or coerced into user/assistant roles. No retry or
live eligibility claim follows from supporting synthetic rows. Unknown future
types still fail closed without echoing their values. Compaction/provenance,
pagination, limits, terminal status and all independent repair guards are unchanged.
The v2 loader requires a default `{id,setup}` definition, not the v1 named
function returning hooks. `@opencode/plugin@2.0.24` is the native typing pin;
v1 SDK pins remain replay compatibility only, not live compatibility claims.
Native setup subscribes to `ctx.event.subscribe`, takes version from
`ctx.app.version` and directory from `ctx.location.directory`, and normalizes
`event.data` into the existing envelope. Inbox user payloads, final text,
terminal tool events and execution success/failure/interruption are observed;
reasoning text is never collected. Stable IDs precede Go task-boundary logic.
`AGF_OBSERVER_SERVER` optionally supplies the validated backfill URL; absence
does not prevent local ingestion. No fake health environment is used by the
native loader. Unload aborts the subscription. Existing opt-in standalone
file ownership/install/sync/uninstall paths remain unchanged.

Fresh first-task lifecycle: location plugin setup can follow session creation.
The native session `prompt` hook captures the actual admitted user message and
reads current session identity from the in-process `ctx.session.get` domain
(flat SessionInfo, not an HTTP data envelope; bounded 2s). This does not recover
historical source content. Every flushed envelope includes cached identity;
bounded identity overflow is explicitly incomplete. Successful read provenance
is captured only in `execute.after`, never by later bus-event file hashing.
Failed reads load no source: preserve their terminal failure as task evidence,
not a malformed pathless `source_gap`. Duplicate terminal tool IDs retain their
observed input/name in a bounded cache.

Flushes serialize per session, detach one snapshot and retain it on ingest
failure ahead of concurrently arriving observations. Sticky terminal triggers
survive an in-flight nonterminal handoff; at most one idempotent terminal retry
uses original stable IDs/bytes. Overflow explicitly marks incomplete coverage.
Non-disabled ingest failures expose a full private redacted stderr diagnostic
locator under `<state>/observer-errors`, without stdin/environment dumps;
disabled verdicts stop staging and create no failure report. No daemon or blind
provider replay is added. A terminal stream with real parts but missing its
opening user turn retains a drain job and explicit incomplete report with a
  locator and any safely derivable local signals, without automatic correction.
  Missing intent prevents semantic interpretation. A bare idle remains degenerate.

Startup completeness is derived from current evidence, not a persisted detail
string. `startup_gaps` stores bounded historical `{message_id,seq}` markers for
the first observable part after the accepted boundary. Drain derives an active
`missing_startup` gap only while that segment lacks its opening user text.
An earlier staged user part (including late role completion) proves ordering;
otherwise an observed assistant `parent_id` must name that exact user message.
Native step-start normalization carries the already observed admitted user ID;
later text events cannot invent the link. The first nonempty observed owner is
preserved on upserts. Native step-start redelivery preserves even an originally
absent owner; queued inbox identity alone is not admitted ownership. Bounded
owner caches refuse new links at capacity rather than rebinding old steps.
An unrelated later prompt/final cannot heal the earlier
segment; absence of ordering/ownership evidence stays incomplete. Healing
supersedes only the active derived gap, preserving historical spool markers and
old diagnostic files, without recreating source content or hashes. Bounded
server reconciliation runs before deriving current completeness. Legacy
unstructured partial-stream gaps remain conservative collection gaps.

Ingest settlement is one-shot across timeout, close, error, missing stdin and
synchronous spawn throw: exactly one full diagnostic per failed attempt, none
for success/disabled. One terminal retry remains unchanged. Native source
exclusions cover basename suffixes `.env`, `.pem`, `.p12`, `.pfx` and `.env`,
credentials/secrets/id_rsa/id_ed25519 path segments (case insensitive), matching
the legacy adapter; excluded files are neither read nor hashed by capture.

- Native runtime versions are OpenCode CLI `2.0.24`, `2.0.27` and `2.0.30`, using the
  unchanged `@opencode/plugin@2.0.24` typing pin.
  `@opencode-ai/plugin@1.18.35` and `@opencode-ai/sdk@1.18.35` are historical
  replay typings, not evidence of live loader compatibility.
- The adapter is version-tested: native setup verifies the loader app version;
  legacy replay probes once. Unsupported generations never stage. SDK
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
- `message` — `{record_id, session_id, role, parent_id?}`; role map (`user`/`assistant`, or `context` paired only with native part type `synthetic`); assistant parent_id is the observed admitted opening user message, never a later inferred prompt. Context never attests this ownership. A synthetic context part with incompatible ownership/legacy ignored flag leaves an explicit collection gap.
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
| `accepted_after_seq` | legacy name for processed-interval cursor; does not certify task success; next interval starts strictly after it |
| `overflows` | count of envelopes dropped by the spool bound |
| `sessions` | observed `{parent_id, title, directory}` per session id |
| `messages` | role per message id |
| `parts` | latest observed part state per part id (upsert, streamed updates replace) |
| `events` | append-only bounded session events (last 400) |
| `source_refs` | deduplicated `{path, sha256, excerpt?}` provenance |
| `gaps` | bounded collection gaps (`compacted`, `partial_stream`, `missing_source`, overflow) |
| `startup_gaps` | bounded historical `{message_id,seq}` missing-opening markers; active gaps re-derived from segment ownership evidence |
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
| legacy v1 synthetic/ignored parts (`synthetic:true`) | not captured (not agent observables); their absence is not a gap |
| native durable `synthetic` input | redacted context message (`role:context, kind:synthetic`); required task context, never a productive step or human task boundary |
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

#### Native execution episodes

Native drain analyzes **input prompt → next observed idle**, not the entire
session or an accepted-success interval. A prompt may come from a person,
another agent or automation; wire role `user` does not establish human authorship.
Additional prompts before that idle remain input to the same episode. Duplicate
idles without new input create nothing; a prompt without idle stays open.
`task.kind:episode` requires positive bounded start/end sequences and reports
`boundary_source:prompt-idle`. Idle establishes closure, never task success;
an assistant final is unnecessary. Explicit errors/interruptions stay distinct.
Legacy offline traces retain their existing host/accepted-success behavior.

The projection retains input, applicable durable nonhuman context, agent tool
calls/results and at-use source hashes. Assistant narrative/private reasoning
is not sent to the model. The native episode decoder excludes harness system/skill
configuration; strict native review/final decoding is unchanged. Validated history
order, including historical idles, supplies a separate redacted snapshot: backfill
arrival sequences cannot establish execution order. Original spools, overflow
counters, collection gaps and reports are preserved, not reset to admit a repair.
Compaction of model context alone does not remove independently fetched stored
prompt/tool rows. Unavailable rows in that snapshot retain positional evidence
gaps; malformed history/pagination or collection caps remain explicit failures.
Unscoped gaps in collector-only fallback remain material to every affected episode.
Descendant sessions have their own episodes; the parent includes its observed
delegating call/result, not the child's entire session. Origin/exposure exclusions
remain authoritative for both roots and descendants.

Existing per-signal neighborhoods are the model chunks: retain the relevant
input and complete selected calls/results; trim optional neighbor calls before
sending. The local union of episode evidence may exceed one HTTP packet, while
each request retains the configured byte/request caps. An indivisible oversized
input/result or signal packet abstains explicitly; no text truncation is treated
as a complete chunk. No model call is made for chunks without signals. Request
budgets apply per episode; deterministic replay uses no provider or dispatch.
Native corrector/reviewer/QA packets keep an oversized report union local and
send a bounded evidence preview plus the authoritative locator. A preview never
replaces required causal reads. Serialized native prompts over the configured
request byte cap are refused before HTTP delivery, including oversized candidates.

Episode identity hashes session, opening input identities and observed idle ID
(staged sequence only when no idle ID exists). The job's `episode_digests` deduplicates unchanged processed input,
without advancing `accepted_after_seq` or inventing task success. An incomplete
earlier episode does not expand across later prompts or prevent their analysis.

#### Retrospective failure feedback

Failed collection/analysis/interpretation/dispatch produces a separate durable
`pipeline-*` maintenance incident, deduplicated by failure class and installed
source root. Its report has `mode:pipeline_failure`, remains incomplete and is
ineligible for ordinary task dispatch/repair. Original report/job locators are
diagnostic evidence, not a fabricated successful task trace.

With an explicit source root inside configured canonical roots, existing enabled
native dispatch/model capability runs the existing read-only corrector followed
by the configured independent reviewer. Only an empty-findings actionable diagnosis
and proposed regression check can become `suggested`; any proposed write is refused.
This path never calls a repair sink. Implementation of executable/hook fixes uses
ordinary scoped change authority. Missing capabilities retain `queued`; ambiguous
attempts retain `pending` without automatic root retries. Learning-origin binding
suppresses capture of diagnosis/review roots and descendants, preventing recursion.

### Drain jobs and lifecycle

`<state>/jobs/<session-id>.json` (schema `1`): `{session_id, server_url?,
status: pending|running|done|failed|skipped, attempts, incidents:{task:
receipt status}, episode_digests?, maintenance_incident?, report_path?, receipt_status?, error?, created_at,
updated_at}`. Skipped covers learning-origin/suppressed spools or absent execution records.
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
  projects closed episodes; validates; runs `analyze` (evaluator budgets per episode/config);
  writes the report into `<state>/reports/`; runs `dispatch` under the same
  global-enable re-check and request budgets; records the receipt status in
  the job and records each processed input digest. Incomplete episodes retain
  reports and fail the job with a separate maintenance incident, while later
  closed episodes still run. Neither closure nor error/abort certifies success.
  Semantic uncertainty and intentionally exhausted budgets remain abstentions,
  not permission to retry forever; explicit re-analysis preserves incident identity;
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

Native message generation now uses the documented **session.create** agent,
Model.Ref and location fields, never ignored prompt extras. Explicit
`dispatch_model` must name an available host profile. Roles are the canonical
`learning-corrector`, configured installed reviewer (distinct from corrector
and `qa-runner`), and `qa-runner`. Each is a separate parentless visible root,
with a learning-origin title/store binding and verified intended directory.
The correcting role investigates report `task_evidence` and source provenance
and returns a typed proposal (at most one Markdown text change) or a completed
advisory. No coordinator finding packet is necessary to execute this workflow.
All native sessions deny direct edits/bash/network; Go alone owns application.
Report `task_evidence` contains bounded redacted signal evidence, task intent,
relevant ordered context and source provenance, with exact coverage/omission
counts. It is not hidden reasoning or a second full-trace provider request.
Missing/omitted causal context can yield advisories but cannot authorize
model-proposed replacements. Independent corrector/reviewer inspection may
verify permitted source evidence, never infer missing historical bytes or
silently promote a truncated report to repair-safe.

Independent review must complete in the actual reviewer role/model and return
an approved verdict bound to the exact candidate SHA. A model proposal alone,
a report-only finding, or the `reviewer` string cannot authorize application.
Collected source path/version, canonical ownership, protected paths, unique
excerpt and fresh on-disk hash gates still apply. Proposed non-Markdown or
unproven causes abstain. QA executes in its own actual role, inspects the
postimage or no-edit advisory, and returns a digest-bound verdict. Failed QA
restores only Go's matching postimage through the existing validator/rollback.
Receipts persist actual results and review/QA root IDs/verdicts; only completed
and verified execution can set `native_completed`. A completed advisory is
`suggested`, never `repaired`. Accepted HTTP delivery alone remains pending.
Native permission rules default-deny all actions. Internal read resources and
grep/glob search patterns are relative strings (`core/src/file-access.ts` and
`core/src/tool/plugin/{grep,glob}.ts`), so read/glob/grep rules allow their
patterns; absolute outside-root targets also require separately scoped
`external_directory` rules for execution/canonical/state roots. Secret-file
patterns are denied last for both relative and absolute paths. No shell,
execute/code runner, edit, network or subagent action is authorized. Packets
declare no shell commands: independent judgments inspect files and causal
evidence using allowed tools; Go owns cryptographic hash computation/gates.
Before **any source mutation**, durably write the preimage and a write-ahead
pending finding containing canonical source/version, planned postimage/diff,
original mode, candidate/reviewer digests and the already-reserved repair attempt.
Sync the journal files and directory entries before replacing the source.
Preimage journal identifiers are single safe path tokens: separators, drive
prefixes and navigation names cannot escape the owned state directory.
Resume accepts only the exact recorded preimage (not yet applied) or postimage
(already applied). A preimage with no QA submission may resume the one reserved
write; an already attempted QA with a preimage is a restored/reverted conflict,
not another application. A postimage resumes executed QA without duplicate roots
or prompts. Missing/mismatched provenance, mode, preimage bytes or unrelated
current bytes conflict without overwrite. Rollback restores only the recorded
matching postimage, including after a crash during validation. Checkpoints at
before-write, after-write, before-validator and after-validator are exercised
through public Dispatch in subprocess-death tests with unrelated-edit controls;
checkpoints are private test seams, not environment/CLI behavior.
Validation rollback re-reads current bytes rather
than using its stale pre-validation snapshot; concurrent user edits survive.
Each stage is bounded by `dispatch_timeout`; timeout interrupts only the owned
root. Lost create/prompt responses reconcile recorded stages without blind
duplicate posting. Actual history must prove agent, selected model, completed
time, no provider error, and visible final text; reasoning content is ignored.

API source: exact v2.0.24 session.create `/api/session`, switch agent/model
routes `/api/session/{id}/agent|model`, and native completion wait
`POST /api/experimental/session/{id}/wait`. No CLI fallback is needed.

`dispatch_endpoint` config key picks the OpenCode server generation:

- `prompt` (default) — the Phase 2 contract exactly: `POST /session` body
  `{title, directory}` (no `parentID` key), prompt `POST /session/{id}/prompt`
  with the marker echo, reconcile by session read.
- `message` — the verified live 2.x contract (empirically confirmed against
  an isolated OpenCode 2.0.24 server; recorded in
  `fixtures/retrospective/opencode/live-api-2.0.24.json`): every route is
  served under the `/api` prefix; requests carry HTTP Basic auth when
  credentials resolve (see below); `POST /api/session` body contains title,
  agent, explicit Model.Ref, location.directory and read-only permissions
  (no parentID). It returns a `{"data": …}` envelope; the session list and
  per-session history are
  `{"data": …, "cursor": {next}}` pages the clients follow to exhaustion
  (bounded; a page limit fails closed); prompt delivery uses the synchronous
  `POST /api/session/{id}/prompt` with the flat body
  `{"text": …}` — the 2xx user-message echo
   confirms delivery only. Native stage reconciliation reads effective
   assistant agent/model, time.completed and final text from message content;
   it does not blindly repost an attempted stage. Legacy marker/history
   reconciliation remains separate. 404/501 queue the
   report exactly as before; no hidden fallback.

Native history rows are a tagged union, not message roles. The literal
`native-history-2.0.24.json` fixture preserves six exact redacted rows from the
synthetic 43716df QA history (two of its 29 tool attempts, user, genuine completed
answer, idle and agent selection); it is not full live learning acceptance.
Authority: pinned `@opencode/schema@2.0.24` SessionMessage schema. Only actual
`user.text` and `assistant.content` become user/assistant messages. Assistant
text is message/ordinal scoped; tools are message/tool-ID scoped and retain
input, status, public error message and textual results. Reasoning bodies,
provider state, control metadata and compaction summaries are never evidence.
Text from unfinished/non-stop/tool-call assistant frames remains observable
but explicitly non-final. The last assistant must contain completed stop text;
idle alone cannot fill a missing final or establish success.

Known idle outcomes project terminal candidate/error/abort events, not roles;
older idles preceding the latest actual user in native chronology cannot become
tail errors merely because backfill arrived later. Agent/model selections have
no task-message projection. Valid location changes/compactions mark incomplete
coverage rather than synthesizing execution identity or assistant text.
Unknown row/content tags, incompatible fields, missing identity/time/content,
unmodeled attachments/results, truncated tool output or malformed state fail
backfill closed with an inspectable gap/report. Native visible fields beyond
the 64KiB record bound fail with a gap before common spool clipping. Core roles remain strictly
user/assistant. Local completeness refusal sets report status incomplete
independently of observed terminal outcome, so drain never advances processed
boundaries for a gap or missing final. Safe local signals remain visible.
Missing intent or material gaps skip semantic actionability and all automatic
correction; abort alone is not a diagnostic gate.

Pagination must exhaust with valid data/cursor fields. Repeated cursors, 200-page
exhaustion, >2,000 raw rows or normalized records, or >8MiB aggregate wire fail
closed; controls count against budgets even though they are not messages. The
shared 8MiB HTTP response cap detects overflow rather than silently clipping a
valid prefix. Pages are ordered by actual created time before staging; streamed
stable IDs retain their first observation sequence. A trusted validated native
control row may remove only the previous adapter's same-ID empty control-as-text
projection; any unexpected old payload stays with a completeness gap. Valid
native message rows also remove old same-ID empty assistant projections and
an exact duplicate user projection only when its original text part is already
present and matches the immutable wire text. Genuine parts/attempts stay. At-use
source refs are never refreshed from current disk content. Legacy history wire,
role validation and origin/auth/redirect/directory gates remain unchanged.
Signal/semantic routing replaces the old numerical waste/success thresholds.

**Credential destination boundary.** Only the configured dispatch-server
origin (scheme, lowercase hostname, effective port) authorizes Basic auth.
Observer stdin and persisted job/spool URLs never grant credential authority.
Collection URLs must be HTTP(S) loopback or the configured hostname, with no
userinfo, query, fragment, or non-root path. Invalid URLs are dropped with a
generic `partial_stream` gap, never echoed into errors or new state. Backfill
independently validates old/persisted URLs and requires the exact configured
origin; mismatch/missing authority yields a gap without network. Another
loopback port or scheme is not trusted. Direct HTTP clients with credentials
must declare `TrustedOrigin`; mutating BaseURL cannot retarget credentials.
All redirects are refused, including custom-client same-host redirects.
Config/base URL parse errors never echo the supplied URL.

Live `message` sessions decode `projectID` and `location.directory` on create,
read, and list. Before any prompt or local repair, root read-back must match
the authorized `dispatch_directory` (else report execution root), as an
absolute cleaned directory; absent/mismatched directory leaves a visible
pending receipt and performs no prompt or repair. Legacy fields stay intact.
The live route delivers text only; accepted extra agent/model fields are
ignored by 2.0.24 and cannot prove role or model selection.

**Authentication.** The live server requires HTTP Basic auth on every `/api`
route (a server started without a password generates a random one, which the
loop cannot learn). A trusted backfill 401/403 is an actionable completeness
gap (configure the server password and restart under that environment), not
a queued dispatch receipt. An ineligible report never fabricates a queued
disposition; eligible dispatch authentication failures remain pending.
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
  (mock session server) and prove: no candidates means no model request;
  candidates send only bounded cited evidence/intent/sequence; justified,
  uncertain, unavailable and incomplete-evidence cases never authorize writes;
  failed/aborted cases retain diagnostics without changing source outcome.
  Guarded actionable cases create exactly one visible correcting root, with
  intake data never interpolated into shell commands. Preserve every existing
  public-path provenance/auth/native-role/WAL/hash/conflict/rollback negative.
- Release/bootstrap fixture tests install the plugin + runtime assets into a
  disposable HOME and exercise enable/disable/sync/doctor/uninstall with
  modified-file retention and no activation of live sessions. Detached
  worker spawns are test-injected (`SpawnWorker` in-process; the CLI honors
  `AGF_RETROSPECTIVE_NO_SPAWN=1`), so fixture tests never launch background
  processes.

### Detection vs repair acceptance

Freeze labelled positive/negative detector fixtures and expected rule/call IDs
before acceptance; include red tests, polling and legitimate broad research as
interpretation controls. Do not repeatedly resample live tasks to force a hit.
Controlled repair acceptance may inject a labelled interpreted-issue fixture
ONLY through a private test-harness seam, never a production flag/env bypass or
fake GLiDE verdict. Prove before-task failure → independent review of exact
proposed diff/digest → guarded repair → identical task success, plus negative
regressions, conflict, rollback, WAL interruption and dedup/origin suppression.
Native corrector/reviewer/QA role/model/completion and visible-root reconnect
evidence remain required; prompt delivery is not completed repair. Publish frozen
candidate/evidence for fresh independent review each phase; supervisor schedules
final QA. A bounded genuine live GLiDE sample is optional, never calibration.
Full natural live end-to-end acceptance remains UNMET when semantic evaluation
was bypassed in the test harness. Report genuine no-hit/uncertain outcomes as-is.
