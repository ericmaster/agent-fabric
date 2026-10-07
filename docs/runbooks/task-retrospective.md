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
  "canonical_roots": ["/absolute/editable/project/roots"],
  "reviewer": "some-independent-reviewer"
}
```

`dispatch_endpoint` is `"message"` for the installed OpenCode 2.x server
(every route under `/api` with HTTP Basic auth — run the server with
`OPENCODE_PASSWORD` set so the loop can authenticate; flat-text prompt
delivery via `POST /api/session/{id}/prompt` + message-history reconcile).
`"prompt"` is the legacy 1.18.x transport default.

4. The drain worker inherits the Jev key from the host environment
   (`TYPESAFE_API_KEY`, read at request time, never persisted) and the
   `AGF_*` environment of the OpenCode server process that spawns it.

The observation plugin is a thin adapter: it converts native events into an
envelope and hands the JSON to `agf retrospective ingest` over stdin. It
records no hidden reasoning, never posts into the source session, and never
blocks a session on Jev. OpenCode must be restarted to load, unload, or
replace the plugin file.

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
