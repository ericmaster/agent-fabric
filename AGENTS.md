# Agent Fabric — Agent Guide

Agent Fabric provides portable, provider-neutral agent definitions and adapters that compile
canonical workflows into native configurations for OpenCode, Kilo, Antigravity CLI, Codex, Claude Code, and Pi.

> [!IMPORTANT]
> **AGENTS.md CRITICAL RULES:**
> **No Fluff:** Minimum characters. Concise but 100% complete.
> **Documentation:** Keep specs and non-ADR docs current-state only, except `CHANGELOG.md`.
> Maintain `CHANGELOG.md` as the release-history SSOT: collect changes under `[Unreleased]`,
> preserve published release entries, and use actual versions/dates; never invent prior releases.
> Keep historical decision rationale in `docs/adr/`; use Git and Plane for commit/task detail.
> **ADR Lifecycle:** Before release, ADRs may be edited, merged, or squashed to retain the latest decision; after
> release, preserve published ADRs and use a new superseding ADR. Full filing rules:
> [`docs/docs-organization-blueprint.md`](docs/docs-organization-blueprint.md).
> **Live Sync:** Keep this file updated with relevant code changes in the same commit.
> **Context Hygiene:** Apply the `context-hygiene` skill before writing or revising any
> documentation or spec; keep each fact in its SSOT and link instead of repeating it.

> [!WARNING]
> **Avoid Redundant Documentation.** AGENTS.md is the Single Source of Truth. Do NOT create
> separate MAINTENANCE.md / ARCHITECTURE.md files that duplicate what is here. Domain
> terminology belongs in [CONTEXT.md](CONTEXT.md); decisions in `docs/adr/`; module
> contracts in `docs/specs/`; command recipes in `docs/runbooks/`. Filing rules:
> [`docs/docs-organization-blueprint.md`](docs/docs-organization-blueprint.md).

## Layout

```
agents/                  Canonical portable agent definitions (schema v1)
adapters/                Target harness adapter JSON mappings
hooks/                   Default hook templates and fallback instructions
hooks/retrospective/     OpenCode observation plugin (thin TS adapter; npm test/typecheck live here)
cmd/agent-fabric/        CLI commands (install, sync, validate, doctor, hub, uninstall, retrospective incl. ingest/drain)
internal/agent/          Agent definition parsing, validation, and hook schema checking
internal/adapter/        Multi-harness rendering engine and format converters
internal/manifest/       Atomic state tracking (.agent-fabric-manifest.json)
internal/retrospective/  Task retrospective: replay, dispatch, observation spool, drain worker
docs/                    Versioned wiki (docs-organization-blueprint.md, specs/, adr/, runbooks/)
fixtures/                Golden test fixtures for adapter rendering (+ retrospective/opencode recorded events/envelopes)
scripts/                 Release packaging and bootstrap tooling
```

## First-time setup

```bash
go test ./...
go build -o agent-fabric ./cmd/agent-fabric
```

## Daily commands

| Command | What it does |
|---|---|
| `go test ./...` | Runs all unit and golden fixture tests |
| `go build ./cmd/agent-fabric` | Compiles the `agent-fabric` CLI binary |
| `scripts/release.sh <version> dist` | Builds cross-platform release archives, checksums, and manifest |

## Architecture at a glance

- **Canonical Definitions as SSOT:** Canonical agent workflows live exclusively in `agents/<id>.md` and contain full portable behavior, permissions, and declarative hook placeholders. Spec: `docs/specs/agent-fabric.md`.
- **Self-Locating Delegation:** Fresh-context handoffs use the fail-closed Delegation Packet contract defined normatively in `docs/specs/agent-fabric.md`; hooks only enrich or validate it.
- **Harness-Isolated Rendering:** Adapters (`adapters/<target>.json` + `internal/adapter/`) map abstract profiles (`planner`, `worker`, `reviewer`, `supervisor`) to harness-specific models and tool syntax without contaminating canonical bodies.
- **Pi:** Optional upstream subagent extension, inherited active model, body-resolved hooks; no native sandbox/continuation or retrospective capture. Setup: `docs/runbooks/pi.md`.
- **OpenCode 2:** Native ordered permissions (`bash`→`shell`, `task`→`subagent`), `model#variant`, `hidden`; portable hooks render as explicit body commands, not plugin frontmatter. Compatibility/rollout: `docs/runbooks/opencode2-supervision.md`.
- **Deterministic Hook Resolution:** Install and sync resolve portable hooks (`load-task`, `pre-plan`, `classify`, `label`, `decompose`, `post-plan`, `record-ledger`, `pre-deploy`, `post-deploy`) once from `~/.agent-hooks/`.
- **Atomic Manifest Ownership:** Project and global installs maintain `.agent-fabric-manifest.json`. Modified user files are preserved during sync unless `--force` is specified.
- **Task Continuity:** Supervisors must pass the recorded child continuation ID on later dispatches. A new session is allowed only for new resolving authority, an approved scope-identity change, or unavailable continuation. Reuse a finding's recorded diagnosis. Hook: `hooks/supervisor/record-ledger.py`. Tests: `python3 -m unittest discover -s tests -p 'test_supervisor_ledger.py'` and `TestSupervisorResumeHandleIsMandatory`.
- **Task Retrospective (opt-in):** One global `retrospective.enabled` switch. Analyze/drain use deterministic friction/report → bounded GLiDE candidate interpretation → independently guarded investigation/repair, never modeled task success or per-call classification. No candidate means no provider request; unavailable/failed tasks retain signals. Controlled Phase 3 tests use real file-task checks and protocol-mocked roles; independent review and supervisor live/UI acceptance pending: `.artifacts-manager/plan-task-retrospective.md`. `--signals-only` and old success/rate/fragment config keys are retired; v1 reports cannot start new repairs. `hooks/retrospective/opencode.ts` hands versioned envelopes to `agf retrospective ingest` over stdin. Terminal triggers persist jobs and launch detached `agf retrospective drain` (single-host lock, daemon-less worker; 2.x reconciliation via `dispatch_endpoint: message`). Learning-origin sessions and descendants never capture. Runbook: `docs/runbooks/task-retrospective.md`. Spec: `docs/specs/task-retrospective.md`. Tests: `go test ./internal/retrospective/`, `npm ci/test --prefix hooks/retrospective`, `python3 -m unittest discover -s tests -p 'test_retrospective_release.py'`.
- **Read-only Preflight:** `python3 hooks/supervisor/support.py` (JSON stdin) reconciles host observations/owned Linux command identities, fingerprints declared actual inputs for verified receipt reuse, and validates compact handoffs. No dispatch, process control or new persistence. Contract: `docs/specs/agent-fabric.md`, Deterministic supervision support. Tests: `python3 -m unittest discover -s tests -p 'test_supervisor_support.py'`.
- **Supervisor Resolution:** Supervisors verify child blocker claims with capability probes and bounded recovery across tools, fixtures, workspaces, Dev state and scoped releases. Final blockers name the exact external-only action and failed probes; unavailable capability/credentials, product intent, explicit policy/scope boundaries and secret disclosure remain limits. Spec: `docs/specs/agent-fabric.md`.
- **Durable Task Authority:** Executable tasks authorize the complete in-scope flow without repeated confirmation, with proportional checkpoint/canary/rollback controls. Plan-only/report-only scope stops execution; release requires a named target, DoD, rollback and traceable authority. Spec: `docs/specs/agent-fabric.md`.
- **Budget Efficiency:** Use the least expensive adequate model, delegation count, and verification set; reuse sessions/evidence and escalate only after objective failure. Never reduce mandatory DoD gates for cost. Spec: `docs/specs/agent-fabric.md`.
- **Outcome-first QA:** Source-design fidelity is separate from screenshot regression; verify a representative page early. Repeated prerequisite recovery has a cross-role no-progress circuit breaker. Contract: `docs/specs/agent-fabric.md` (Outcome-first verification and bounded recovery).
- **Visible Progress:** Early functional smoke → provisional isolated implementation; broader checks/review run on frozen inputs in background. No Done/integration/deployment before acceptance. Contract: `docs/specs/agent-fabric.md` (Bounded episodes, waiting, and reduced delivery).
- **Compact Context:** One current packet and report locators; retain loaded checkpoint, use record receipts and status-only `compact: true` loads. Full load remains required for recovery/decisions. Contract: `docs/specs/agent-fabric.md` (Continuity and checkpoints).
- **Crash-safe Resume:** Resume-critical checkpoints, candidates and receipts use a durable private root; temporary storage is scratch. Missing evidence is not PASS and does not reset budgets. Contract: `docs/specs/agent-fabric.md` (Bounded episodes, waiting, and reduced delivery).
- **Phase Closure:** Validate → independent review → scoped commit/integration → verified durable receipt → eligible owned cleanup; compact planning decisions, preserve unresolved evidence. Contract: `docs/specs/agent-fabric.md` (Phase closure and compact planning).
- **Discretionary QA:** Recommended, not mandatory; loop-supervisor decides run/skip/reuse/defer. QA-only execution blockers go upward without freezing implementation; unrun checks never become PASS. Setup changes belong to implementation roles under existing authority. Spec: `docs/specs/agent-fabric.md` (Discretionary QA Contract).
- **Delivery & Recovery:** Supervisors own integration, authorized deployment, verification and clean closure. Soft exhaustion triggers evidence-backed bounded recovery under standing task authority, not repeated permission requests; preserve history and explicit hard caps. Prefer a single-writer current checkout after dirty-work triage; isolate concurrent writers or when mandated. Never discard/commit foreign work. Contract: `docs/specs/agent-fabric.md` (Delivery ownership and checkout closure; Native asynchronous orchestration).

- **Reusable lessons:** Existing private Store supports signed `lessons apply/read/status` and explicit `lessons generate --config ABS --trust ABS --input ABS`: existing native corrector/configured independent reviewer, effective role/model/completion and digest-bound typed acceptance; no new keys, model/permission changes or automatic generation. Natural repeated input contrasts resolve observed record hashes; single/confounded contrasts abstain. Native receipts + safe negative trace bytes remain durable; public apply cannot forge native authority. Optional v2 `context` uses operator per-session `AGF_LESSONS_CONTEXT`, global/scoped enablement and advisory executor-only delivery receipts. Exclusions, learning/exposure origins and known descendants suppress generation/admission. Other harnesses use explicit read; `load-task` unchanged; no legacy reads/global promotion. Contract/runbook: `docs/specs/task-retrospective.md` and `docs/runbooks/task-retrospective.md` (Reusable lessons). CLI fixtures: `go test -count=1 -v ./cmd/agent-fabric -run '^TestLesson(Generation)?CLI$'`; registered-hook fixture: `npm test --prefix hooks/retrospective`. Protocol mocks/signing/callbacks do not prove live review/provider consumption; phase6 native E2E remains separate.

## Testing

- Unit tests in `internal/agent`, `internal/adapter`, `internal/manifest`, `internal/retrospective`, and `cmd/agent-fabric`.
- Golden fixture comparison in `internal/adapter/adapter_test.go` checks rendered output against `fixtures/`.
- Retrospective uses Fastino GLiDE by default (`FASTINO_API_KEY`, `/v1/systemone`); Jev is unavailable pending verified contract. Default tests use local mocks/synthetic traces, never real provider keys. Explicitly authorized `TestRealEpisodeReplay` reads native history into private QA output with no provider calls or live-store writes. Live prompt delivery is not autonomous-correction QA.
- Native first-task capture uses prompt-hook identity/intake and at-use read hashes. Failed read events never hash a later file. Serialized flushes retain failed batches and surface full private diagnostic locators; missing-startup terminal work keeps an incomplete job/report, never a forced eligible trace.
- Native lesson readiness: bounded same-file read brackets and prompt/read Git HEAD observations survive exact native-history identity joins; missing/conflicting/gapped evidence abstains. Contract: `docs/specs/task-retrospective.md` (Narrow native read brackets); collector→reflection regression: `npm test --prefix hooks/retrospective`. Bracket stability is not no-transient-mutation or causal-benefit proof.
- Native durable synthetic input is retained as redacted nonhuman `synthetic_context` evidence, never human intent, an assistant action/final, a task boundary or source provenance. Opaque metadata must be a non-null JSON object and is not forwarded. Legacy v1 synthetic-part exclusions are separate.
- Gap coverage uses explicit host/prompt-idle/accepted-success intervals and observed gap ranges. Historical gaps remain visible with stable IDs; current/unknown scope remains material incomplete. Follow-ups never invent task completion. Contract: `docs/specs/task-retrospective.md` (Historical versus current collection gaps).
- Native episode diagnosis: prompt (human/agent/automation) → observed idle; ordered prompt/tool projection and per-signal model packets, never accepted task success. Retrospective failures queue deduplicated advice-only native investigation/review, never executable self-repair. Contract: `docs/specs/task-retrospective.md` (Native execution episodes; Retrospective failure feedback). Checks: `go test ./internal/retrospective`; opt-in real replay: `TestRealEpisodeReplay`.
- Native digest review: host-versioned packet marker selects the canonical reviewer's typed verdict; pipeline advice requires observed implementation HEAD, not original-task provenance. Contract: `docs/specs/task-retrospective.md` (Retrospective failure feedback); checks: `TestNativeDigestReviewRoleAndPackets`, `TestPipelineProtocol`.
- Gap retention deduplicates exact owner/kind/record/range observations before the 800-row cap; any known distinct evidence eviction persists a sticky unknown `coverage_overflow` outside the cap. Changed sequences/owners remain distinct. Never clear loss counters to enable repair.
- Retrospective monitoring: `scripts/retrospective-monitor.py --state-dir <absolute-store>` is read-only inventory; verified scheduled IDs retain separate health without capture exemptions. Opt-in archive-first cleanup accepts exact run receipts or a distinct legacy composite (three allowlisted tasks, complete bounded native discovery, exact current stored prompt/native user.text hash); never title/manual-ID authority. Verified classification is fsynced in the private Python cursor BEFORE the destructive adapter phase; adapter failures block dispatch, preserving real pending work. Closed scheduled knowledge/assist metadata and nullable native cursors are supported; continuations/children/queue work retain. Hourly gate activation/rollback: `docs/runbooks/retrospective-monitor.md`. Spec: `docs/specs/retrospective-monitor.md`. Tests: `python3 -m unittest discover -s tests -p test_retrospective_monitor.py`; `node --test tests/test_monitor_cleanup.mjs`.
- Native binding: before installation or observer-failure diagnosis, read the explicit `AGF_BIN` prerequisites/coverage limits in `docs/runbooks/task-retrospective.md`; enforcement contract: `docs/specs/task-retrospective.md` (Installed version pins).
- Monitor archive anchor/output defaults follow the executing script's checkout, not CWD or caller paths; ancestor safety and deployed state/gate defaults remain unchanged. Regression: `node --test tests/test_monitor_cleanup.mjs`.
- Retrospective Basic auth is bound to the configured dispatch origin (scheme/host/effective port); observer/persisted URLs cannot grant authority. Native 2.0.24 uses the default `{id,setup}` plugin definition and `@opencode/plugin` types, not v1 hook loading. Correcting/reviewer/QA roots select role/model at session creation; text-only prompt extras grant no selection. Directory, effective history role/model, digest-bound independent verdicts and completed results gate repair. Explicit `dispatch_model` required; see the runbook for clean disposable readiness QA.
- The retrospective hooks package (`hooks/retrospective/`) owns the npm/TS tooling: `npm ci --prefix hooks/retrospective`, then `npm test --prefix hooks/retrospective` and `npm run typecheck --prefix hooks/retrospective` are deterministic and offline after ci. Native versions are explicitly allowlisted `2.0.24`/`2.0.27`/`2.0.30`/`2.0.31` with capability checks; typing pin stays 2.0.24. Recorded OpenCode event/envelope fixtures are shared with the Go replay tests in `fixtures/retrospective/opencode/`. Current compatibility matrix, no-provider evidence and remaining acceptance: `.artifacts-manager/agf1-current-runtime-compat-handoff.md`.
- Release/bootstrap fixture tests: `python3 -m unittest discover -s tests -p 'test_retrospective_release.py'` (local fake curl; no network, no real OpenCode server).
- Run `go test ./...` before committing.

## Secrets

Zero secrets, API keys, or provider tokens are tracked in this repository. Authentication is host-managed.

## Specs

Module and CLI behavioral contract lives in [`docs/specs/agent-fabric.md`](docs/specs/agent-fabric.md). Behavior change = update spec in the same commit as code.
