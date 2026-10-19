---
title: "Runbook: Task Retrospective (OpenCode observation loop)"
type: runbook
status: active
covers: cmd/agent-fabric (retrospective ingest/drain), hooks/retrospective, internal/retrospective (spool/jobs/drains)
last_checked: 2026-10-15
---

# Task Retrospective — setup, disable, status, recovery

The loop observes OpenCode sessions and stages bounded event records locally.
The approved replacement records deterministic friction even on failed/aborted
tasks; only candidate evidence goes to compact GLiDE interpretation. No candidate
means no model request. A signal is not proof of avoidability or permission to
repair. Justified/uncertain/unavailable results stay visible; complete causal
evidence and independent review/QA still gate writes. Contract:
[`docs/specs/task-retrospective.md`](../specs/task-retrospective.md)
(observation section). Everything is one explicit global opt-in; nothing runs by
default.

**Controlled acceptance:** default analyze/drain use the single signal-first pipeline.
Phase 3 fixtures test file-task failure/repair/success with protocol-mocked native
roles (`go test ./internal/retrospective -run TestControlledNativeCatalogRepair -v`)
and frozen public CLI observations (`go test ./cmd/agent-fabric -run TestFrozenSignalsPublicCLI -v`).
Mocked role replies are NOT live independent reviewer/QA evidence. Independent
review and supervisor live/UI acceptance are pending; nothing is installed or
activated by this change. The [canonical plan](../../.artifacts-manager/plan-task-retrospective.md)
owns remaining acceptance. Replay with `agf retrospective analyze --input
trace.json --output report.json`; no candidate means no provider request, and
missing provider leaves useful signals. The former `--signals-only` flag and
success/rate/tail/fragment config keys produce explicit migration errors.
Use the spec's flat signal keys and `minimum_actionability_probability` /
`minimum_actionability_confidence` instead. V1 reports stay readable and unchanged,
but cannot start new repairs; re-analyze original evidence. Only an existing exact
native WAL intent can resume under its recorded hash/reviewer/QA safeguards.
Analysis sets `repair_safe:false`; semantic dispatch is an investigation, not
permission for a detector/model-only source write.

## Setup

1. Install Fabric with the opt-in assets (bootstrap or install):

```sh
agf install --all --tools opencode --assets retrospective --yes
```

This owns exactly two files (both manifest-tracked, so project `--project …`
scopes set the project plugin path instead):

- the observation plugin: `~/.config/opencode/plugins/retrospective.ts`
  (project scope: `<project>/.opencode/plugins/retrospective.ts`)
- the runtime asset: `~/.config/agent-fabric/retrospective/learning-origin.json`

2. Enable the single global switch (writes only `retrospective.enabled` in
   `~/.config/agent-fabric/config.json`, preserving other keys):

```sh
agf retrospective enable
```

3. Configure the retrospective knobs once
   (`$XDG_CONFIG_HOME/agent-fabric/retrospective.json`, else
   `~/.config/agent-fabric/retrospective.json`; `AGF_RETROSPECTIVE_CONFIG`
   overrides the path). Keys and defaults live in
   `internal/retrospective/config.go`; the ones the loop needs:

```json
{
  "dispatch_server": "http://127.0.0.1:4096",
  "dispatch_ui_template": "https://<ui-host>/{id}",
  "dispatch_endpoint": "message",
  "dispatch_model": "<available-provider>/<available-worker-model>",
  "canonical_roots": ["/absolute/editable/project/roots"],
  "reviewer": "code-reviewer"
}
```

`dispatch_endpoint` is `"message"` for the installed OpenCode 2.x server
(every route under `/api` with HTTP Basic auth — run the server with
`OPENCODE_PASSWORD` set so the loop can authenticate; flat-text prompt
delivery via `POST /api/session/{id}/prompt` + message-history reconcile).
The Basic credential destination is exactly the configured server origin
(scheme + hostname + effective port). Observer or persisted URLs cannot
retarget it, even to another loopback port. Invalid collection URLs leave a
gap; mismatched backfill origins send no request. Never put credentials,
queries, fragments or non-root paths in a server URL.
`"prompt"` is the legacy 1.18.x transport default.

4. The drain worker inherits the Fastino key from the host environment
   (`FASTINO_API_KEY`, read at request time, never persisted) and the
   `AGF_*` environment of the OpenCode server process that spawns it.
   GLiDE is the default (`evaluator: "glide"`); Jev is unavailable pending
   verification of its current contract. For live Fastino requests consider
   `http_timeout: "300s"` (provider recommendation); retries remain bounded.

   The request remains pinned to `fastino/GLiDE`. Responses accept exactly
   `glide` and observed-live `glide_v2`; the latter is not an officially listed
   alias (OpenAPI says string/example `glide`, the guide still expects `glide`).
   Report model metadata names the requested model, not the response engine.
   Unknown versions fail closed with `unknown_provider_model`; do not widen the
   allowlist from spelling similarity or treat a model marker as authentication.
   Captured evidence: `.artifacts-manager/agf1-glide-model-discriminator.md`.

The observation plugin is a thin adapter: it converts native events into an
envelope and hands the JSON to `agf retrospective ingest` over stdin. It
records no hidden reasoning, never posts into the source session, and never
blocks a session on evaluation. OpenCode must be restarted to load, unload, or
replace the plugin file.

2.0.24/2.0.27/2.0.30 prompt extra agent/model keys are ignored: prompt sends text only.
Selection instead uses native session.create fields `agent`, `model` (Model.Ref)
and `location`. The canonical learning-corrector investigates a typed proposal;
separate visible parentless reviewer and qa-runner roots execute before acceptance.
Direct model edits/bash/network are denied. The receipt records actual visible
final results, role/model evidence and digest-bound verdicts, not delivery alone.
Missing/unavailable profiles, incomplete history or verdicts remain pending.
A completed no-edit advisory is `suggested`, not `repaired`.

The native default plugin definition uses installed-generation
`@opencode/plugin@2.0.24`, `ctx.event.subscribe` and `ctx.app.version`.
Its v1 named hook export is replay compatibility only. The runtime allowlist is
exactly OpenCode `2.0.24`, `2.0.27` and `2.0.30`; required prompt/get, execute.after and event
subscription capabilities are checked before staging. Other versions (including
future patches) fail closed and log a full private diagnostic under
`<state>/observer-errors`. Do not widen the allowlist from `/api/info` alone.
Exact-tag comparison found only one consumed history addition: optional structured
idle `error`, validated locally without copying provider bodies or changing roles.
Set
`AGF_OBSERVER_SERVER` to the exact configured dispatch origin for backfill;
without it local native events still stage, but historical backfill has no URL.
Never use `AGF_PLUGIN_FAKE_HEALTH` in a live server. Native `AGF_BIN` must be an
explicit absolute versioned packaged Go executable, not a symlink or CWD/HOME
fallback. File and immediate parent must be current-uid-owned and not group/world
writable. Keep release bytes immutable while active. Setup probes ingest `--help`
once (5s/output bounded), before hooks; missing/incompatible bindings decline with
one full private diagnostic and no ingest children. Check diagnostic observer PID,
CLI, resolution source, timestamp and declared origin (not verified attribution).
An unbound OpenChamber context is inert; successful activation on 4097 alone does
not establish OpenChamber/all-session coverage. Changed release bytes decline until
reload; transient ingest retries are unchanged. Native call IDs are assistant-message scoped because provider
IDs repeat across turns. Source hashes come from actual terminal read hooks.

## Clean disposable native readiness reproducer

Compatibility matrix: 2.0.24 and 2.0.27 retain recorded execution fixtures;
2.0.30 additionally has actual installed default-loader/setup/hook registration,
public session creation/role/model controls, imported synthetic history, SSE
queued prompts (`resume:false`) and real Go ingest evidence. **No provider or
actual generation/tool execution occurred in the 2.0.30 check.** Its history
fixture is the literal public response after importing a labelled synthetic
transcript, not a new successful task. Event fixture projects only `type/data`
from its actual public SSE; the replay terminal trigger is explicitly synthetic.
Report: [current runtime compatibility](../../.artifacts-manager/agf1-current-runtime-compat-handoff.md).
For the next installed upgrade: use Context7 for orientation, compare exact-tag
loader/Promise hooks/session/event/history schemas, pin installed bytes before
and after, rerun disposable default loader/public schema and CLI-help probes,
control/malformed and legacy tests, then independently review an explicit version
addition. Never allow arbitrary patches or infer a running API version from a
replacement package. Preserve user model overrides; no automatic downgrade.

Use only a disposable root and synthetic evidence, never the host HOME or user
4097/4098 service. Build the CLI to scratch, provision disposable HOME/XDG
config/state/data/cache and project directories, and start the installed
`opencode serve --hostname 127.0.0.1 --port <free-port>` from that project with
an allowlisted environment (`env -i`): PATH, HOME, PWD, XDG paths, synthetic
OPENCODE_PASSWORD, AGF_SOURCE, AGF_BIN, AGF_CONFIG,
AGF_RETROSPECTIVE_STATE/CONFIG, AGF_OBSERVER_SERVER. Do not inherit host
OPENCODE config overrides/provider credentials. For observation-only readiness,
omit all evaluator credentials and use a no-candidate task. Do not rely on
`AGF_RETROSPECTIVE_NO_SPAWN=1` alone: the current CLI has a separate terminal-job
launch path; a bounded worker may still finish the deterministic local report.
Retain its actual job/report and confirm skipped interpretation, not a live
provider result. Correcting this separate spawn control is outside version support.
Use `XDG_CONFIG_HOME=$HOME/.config`: the installer writes there via HOME. Keep
the disposable project outside any existing repository/config ancestor.
Install via `agf install --all --tools opencode --assets retrospective --yes`,
enable with `agf retrospective enable`, create `docs/instructions.md` with a synthetic
redaction canary, and run bounded native CLI:

```sh
timeout -k 10s 240s opencode run --server http://127.0.0.1:<free-port> \
  --model opencode/fledge-alpha-free --agent build --auto \
  --title 'AGF native observer readiness' \
  'Use the read tool to read docs/instructions.md and reply exactly QA_OBSERVER_DONE. Do not edit files.'
```

The model above is a disposable readiness profile, not a canonical-agent pin
or a guarantee of future availability. Require successful actual result and
redacted spool **and** job. Test default plugin loading, not a manual shim.
For dispatch readiness provide an explicitly synthetic eligible report and
configured available model; require `native_completed`, real canonical
learning-corrector history, independent reviewer/QA histories and final verdicts.
Keep synthetic report probes distinct from native end-to-end learning acceptance.
Bound every child and HTTP call; stop only an owned server after checking UID,
PID and cwd. Retain public final text only, never hidden reasoning.

2.0.27 implementation readiness observed actual default plugin loading, prompt
identity/user intake, terminal read, at-use instruction hash and a terminal job
with public `opencode/nemotron-3.5-super-vl-free` in a clean disposable runtime.
That model was selected from the isolated public model snapshot, not installed
as a canonical/default model. No GLiDE call, native independent repair or UI
acceptance is claimed. Exact commands, setup corrections, cleanup and evidence:
[`agf1-native-2027-handoff.md`](../../.artifacts-manager/agf1-native-2027-handoff.md).
The 2.0.27 event fixture is a subset of that actual SSE stream (reasoning omitted);
session/history fixtures are literal public HTTP responses (reasoning text empty).
Failed-idle-error regressions are explicitly schema-derived synthetic controls.
The Go provenance filter retains instruction basenames/trees, not arbitrary
`instructions.md` at project root; use `docs/instructions.md` for the hash check.

`fixtures/retrospective/opencode/events/native-2.0.24.jsonl` is projected from
the actual isolated native SSE stream: keep only `type` and `data` of observable
events, remove all reasoning events, and redact the synthetic canary. Its source
command is the reproducer above. The npm native default-loader replay also
adds a clearly synthetic repeated-call-ID negative control. This fixture is not
proof of a live GLiDE calibration or a complete autonomous stale-runbook loop.
Controlled detection and repair acceptance are separate from a natural live
learning loop. Freeze positive/negative detector fixtures; controlled repair may
inject a labelled interpreted issue only inside a test harness. Prove task
failure, exact-diff independent review, repair, repeat-task success and negative
regressions/conflict/rollback. Native role/model/completion and browser/reconnect
checks remain required. A bounded genuine live GLiDE sample is optional, not
calibration; full natural live e2e stays UNMET if semantic evaluation was bypassed.

## Scoped disable (without a global off)

A `.retrospective-disabled` empty marker file in a project directory (or any
ancestor of it) disables capture for sessions rooted there. Use it when the
global flag is on but one workspace must not be observed.

## Status and inspection

```sh
agf retrospective status                 # enablement, receipts, jobs, spools
ls ~/.local/state/agent-fabric/retrospective/spool   # staged sessions
ls ~/.local/state/agent-fabric/retrospective/jobs    # one job per session
cat ~/.local/state/agent-fabric/retrospective/jobs/<session>.json
```

State default: `AGF_RETROSPECTIVE_STATE`, else
`$XDG_STATE_HOME/agent-fabric/retrospective`, else
`~/.local/state/agent-fabric/retrospective` (dirs 0700, files 0600).
Receipts keep the report locator, the visible root id, and the UI URL.
Reports/traces under `reports/` and `traces/` are retained 30 days; finding
rows until the source hash changes.

## Explicit reusable lesson lifecycle (offline seam)

No installation, service, provider request or legacy import is needed. Explicit
signed ingestion/read/status is not automatic generation or context injection.
Observer/global enable behavior is unchanged; `lessons apply` is an explicit
operator mutation, not a background capture path.

Provision operator-controlled trust JSON outside candidate-controlled roots:
schema 1, `identities` mapping producer/reviewer IDs to `role`, base64 Ed25519
`public_key`, exact absolute `project`. Keep private keys out of AGF state,
repository, packets and logs. Distinct roles backed by one key are rejected.
Trust enrollment comes from the operator, never the proposal. Revoking a key
fails closed for its persisted lessons; retain old public-key enrollment when
old accepted lessons must remain readable.

Candidate/evidence schemas, digest serialization and domain-separated signature
messages: `docs/specs/task-retrospective.md`, Reusable lessons. The independent
signer inspects the candidate and complete cited trace, then signs its exact
digest. Prose PASS, a caller-provided role, or a different signed candidate is
insufficient. Cryptographic identity does not prove review quality or native
model execution.

```bash
agf retrospective lessons apply --state-dir /absolute/private/retrospective --trust /absolute/operator/trust.json --input /absolute/evidence/proposal.json
agf retrospective lessons read --state-dir /absolute/private/retrospective --trust /absolute/operator/trust.json --project /absolute/project --scope catalog-task --version exact-source-revision --consumer executor --limit 10
agf retrospective lessons status --state-dir /absolute/private/retrospective --trust /absolute/operator/trust.json
```

Use `consumer corrector` separately for improvement evidence; neither consumer
grants task/DoD/permission authority. Read/status need no enablement/config,
create no state, and never call a model. Refine/retire requires the current
revision; retired IDs cannot revive. Full capacity rejects writes, without
eviction, promotion or historical import. Preserve corrupt `lessons.json` for
diagnosis; never replace it with an empty document. The snapshot retains signed
envelopes and exact evidence bytes, surviving removal of disposable inputs.

Synthetic-key real-CLI lifecycle, concurrency, write-fault, restart and rejection
fixtures (not native/provider acceptance):
`go test -count=1 -v ./cmd/agent-fabric -run '^TestLessonCLI$'`.

## Recovery

- **Read-only monitor presentation:** use the reviewed metadata helper and
  [scheduler prompt block](retrospective-monitor.md), not ad hoc row/counter sums.
  Helper/prompt rollout is separate from Go-core wrapper pin activation.

- Gap retention: identical owner/kind/record/range observations share a row
  before the 800-row cap, with additional occurrence counts. Changed range or
  owner stays distinct. Any known row/part/compaction evidence eviction creates
  sticky `coverage_overflow` (unknown) from fixed persisted loss counters. Never
  clear those counters or treat them as historical to enable repair. Legacy
  missing counters cannot prove the absence of past loss.

- **Historical versus current gap alerts.** Read `trace.boundary_source` and
  `trace.interval` before calling a gap historical. Reports classify structured
  gaps as `historical`, `current`, `outside_interval`, or `unknown`; unknown scope
  still blocks semantic/write eligibility. A follow-up is not proof of a new task.
  Count stable gap IDs distinctly across reports, not repeated rows; unidentified
  legacy gaps remain separate unknown occurrences. Old report bytes/state are not
  rewritten. Current missing provenance still blocks writes; no new file hash or
  inferred success may clear it. Whole-session legacy reports with no accepted or
  explicit host boundary remain honestly incomplete. Monitor/UI projection is a
  separate change, not activated by this reporting contract.

- **Native history control/unknown-row failure.** Native idle/agent/model
  selections are not user/assistant messages. Inspect the retained job/report
  and redacted history. Valid snapshot normalization preserves actual tool
  attempts and the completed final; malformed/unknown content leaves a gap
  with no automatic repair or processed-boundary advance. The replacement
  retains safe local signals; missing intent/material gaps skip semantic
  actionability. Do not invent a role,
  final answer or historical source hash, or force a job eligible. Existing
  same-ID empty control projections can reconcile from trusted valid history;
   unexpected payloads remain incomplete. A supervisor owns any live retry.
   OpenCode 2.0.30 `synthetic` rows are durable session input: validated public
   text is retained as explicitly nonhuman `synthetic_context` evidence under
   the same redaction/caps. Missing context cannot yield complete coverage.
   It is not a human task boundary, assistant action or completed answer. Opaque
   metadata (when present, a non-null JSON object) and display descriptions are
   never forwarded; malformed metadata fails closed. A native
   synthetic part's reasoning flag always withholds its body and leaves material
   incomplete coverage, including late flags and descendants. Replay/direct input
   uses the same shared context redactor as native capture; spool redaction alone
   is not evidence that replay is safe. A native
   `system`, `skill`, or `shell` diagnostic still identifies material unsupported
   evidence; do not drop those rows, relax guards, or repeatedly retry to force
   eligibility. Unknown type values/private row bodies are never diagnostic text.

- **Opening observations/ingest batch missing.** Failed reads are terminal
  task evidence, not source loads. Native prompt intake obtains current identity
  before generation; read hashes must come from the actual successful read hook,
  never today's file as a substitute for a past read. Ingest failures retain
  the original batch, serialize later flushes and allow one terminal retry.
  Server stderr names the full private `<state>/observer-errors/<id>.json`
  diagnostic (no transcript/environment dump). Missing-startup terminal work
  retains a job and incomplete report; replacement local signals do not permit
  semantic actionability or repair without intent/complete evidence. Do not invent
  source hashes, user turns or manual jobs to make the report eligible.

- **Process died during a native repair.** A durable write-ahead pending finding
  is saved and synced before source replacement, with preimage/postimage,
  canonical version/mode, reviewer/candidate digests and reserved attempt.
  Dispatch resume recognizes only those exact bytes. An unapplied preimage can
  finish the reserved write once; a postimage resumes the existing QA stage.
  Already attempted QA plus a preimage indicates restoration/reversion, not
  permission to reapply. Changed bytes/mode/provenance conflict without overwrite.
  Failed QA restores only the matching owned postimage. Do not delete the journal,
  reset attempts, overwrite unrelated edits or create replacement worker roots.

- **Backfill authentication rejected (401/403).** Configure the trusted
  server password and restart with a matching environment; random generated
  server passwords are not discoverable by the loop. This is an actionable
  completeness gap, not a queued dispatch receipt. Ineligible reports do not
  create receipts. An eligible dispatch rejected by auth remains pending.

- **A drain worker crashed.** Jobs left in `running`/`pending` are resumed by
  the next trigger, or start one now: `agf retrospective drain`. A second
  worker exits immediately (single-host lock, `drain.lock`).
- **The loop is disabled mid-flight.** Paused jobs stay `pending` with their
  reason; `agf retrospective enable` + `agf retrospective drain` resumes
  them. No work is lost; queued receipts stay visible.
- **The correcting-session capability was missing.** The receipt is `queued`
  with the actionable error. Fix `dispatch_server`/`dispatch_ui_template`,
  then rerun the drain or `agf retrospective dispatch --report <file>
  --retry`. Nothing was posted; there is no hidden fallback.
- **A prompt/create response was lost.** The receipt is `pending`. Reconcile
  via the UI (search the session titles for the marker) or
  `agf retrospective dispatch --report <file> --retry` (adopts one exact
  marker match; never re-posts blindly).
- **A queued/pending receipt for the same task.** The drain refuses a second
  root for one task identity: the job fails with "no duplicate root". Resolve
  the receipts first.
- **Suppressed or wrong sessions.** Sessions listed in
  `origins/<session-id>` (any correcting runs of this loop), sessions whose
  title contains `agf-learning-origin:v1:`, and all their descendants are
  never captured; their staged spool is deleted when discovered.
  `origins/` is authoritative.
- **Rollback of a repair.** `dispatch` records preimage/postimage hashes.
  `agf retrospective` receipts point at the report; edits restore only when
  the current hash still matches the repair output.

## Disable and uninstall

```sh
agf retrospective disable    # no network, no mutation; staged work pauses
agf uninstall --all --tools opencode --assets retrospective --yes
```

Uninstall removes only unchanged owned files; user-modified plugin/asset
files are kept and reported. Bytes of unrelated plugins are never touched.
Staged spools/jobs/receipts can be inspected or deleted under the state
directory after disabling.
