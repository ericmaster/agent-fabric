---
title: "Runbook: Task Retrospective (OpenCode observation loop)"
type: runbook
status: active
covers: cmd/agent-fabric (retrospective ingest/drain), hooks/retrospective, internal/retrospective (spool/jobs/drains)
last_checked: 2026-10-07
---

# Task Retrospective — setup, disable, status, recovery

The loop observes OpenCode sessions, stages bounded event records locally,
and — after a confident success verdict — dispatches one visible correcting
root on the configured OpenCode server. Contract:
[`docs/specs/task-retrospective.md`](../specs/task-retrospective.md)
(Phase 3 section). Everything is one explicit global opt-in; nothing runs by
default.

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

The observation plugin is a thin adapter: it converts native events into an
envelope and hands the JSON to `agf retrospective ingest` over stdin. It
records no hidden reasoning, never posts into the source session, and never
blocks a session on evaluation. OpenCode must be restarted to load, unload, or
replace the plugin file.

2.0.24 prompt extra agent/model keys are ignored: prompt sends text only.
Selection instead uses native session.create fields `agent`, `model` (Model.Ref)
and `location`. The canonical learning-corrector investigates a typed proposal;
separate visible parentless reviewer and qa-runner roots execute before acceptance.
Direct model edits/bash/network are denied. The receipt records actual visible
final results, role/model evidence and digest-bound verdicts, not delivery alone.
Missing/unavailable profiles, incomplete history or verdicts remain pending.
A completed no-edit advisory is `suggested`, not `repaired`.

The native default plugin definition uses installed-generation
`@opencode/plugin@2.0.24`, `ctx.event.subscribe` and `ctx.app.version`.
Its v1 named hook export is replay compatibility only. Set
`AGF_OBSERVER_SERVER` to the exact configured dispatch origin for backfill;
without it local native events still stage, but historical backfill has no URL.
Never use `AGF_PLUGIN_FAKE_HEALTH` in a live server. `AGF_BIN` must locate the
packaged Go CLI. Native call IDs are assistant-message scoped because provider
IDs repeat across turns. Source hashes come from actual terminal read hooks.

## Clean disposable native readiness reproducer

Use only a disposable root and synthetic evidence, never the host HOME or user
4097/4098 service. Build the CLI to scratch, provision disposable HOME/XDG
config/state/data/cache and project directories, and start the installed
`opencode serve --hostname 127.0.0.1 --port <free-port>` from that project with
an allowlisted environment (`env -i`): PATH, HOME, PWD, XDG paths, synthetic
OPENCODE_PASSWORD, AGF_SOURCE, AGF_BIN, AGF_CONFIG,
AGF_RETROSPECTIVE_STATE/CONFIG, AGF_OBSERVER_SERVER. Do not inherit host
OPENCODE config overrides/provider credentials. For observation-only readiness,
`AGF_RETROSPECTIVE_NO_SPAWN=1` avoids evaluator network while still staging jobs.
Install via `agf install --all --tools opencode --assets retrospective --yes`,
enable with `agf retrospective enable`, create `instructions.md` with a synthetic
redaction canary, and run bounded native CLI:

```sh
timeout -k 10s 240s opencode run --server http://127.0.0.1:<free-port> \
  --model opencode/fledge-alpha-free --agent build --auto \
  --title 'AGF native observer readiness' \
  'Read instructions.md and reply exactly QA_OBSERVER_DONE. Do not edit files.'
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

`fixtures/retrospective/opencode/events/native-2.0.24.jsonl` is projected from
the actual isolated native SSE stream: keep only `type` and `data` of observable
events, remove all reasoning events, and redact the synthetic canary. Its source
command is the reproducer above. The npm native default-loader replay also
adds a clearly synthetic repeated-call-ID negative control. This fixture is not
proof of a live GLiDE calibration or a complete autonomous stale-runbook loop.
Browser/reconnect, live GLiDE calibration and full native learning-loop QA remain
separate mandatory unmet verification until actually executed.

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

## Recovery

- **Opening observations/ingest batch missing.** Failed reads are terminal
  task evidence, not source loads. Native prompt intake obtains current identity
  before generation; read hashes must come from the actual successful read hook,
  never today's file as a substitute for a past read. Ingest failures retain
  the original batch, serialize later flushes and allow one terminal retry.
  Server stderr names the full private `<state>/observer-errors/<id>.json`
  diagnostic (no transcript/environment dump). Missing-startup terminal work
  retains a job and incomplete report and cannot evaluate/repair. Do not invent
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
