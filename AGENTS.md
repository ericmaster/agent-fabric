# Agent Fabric — Agent Guide

Agent Fabric provides portable, provider-neutral agent definitions and adapters that compile
canonical workflows into native configurations for OpenCode, Kilo, Antigravity CLI, Codex, and Claude Code.

> [!IMPORTANT]
> **AGENTS.md CRITICAL RULES:**
> **No Fluff:** Minimum characters. Concise but 100% complete.
> **No History:** No changelogs. Reflect ONLY the current "Source of Truth".
> **Live Sync:** Keep this file updated with relevant code changes in the same commit.

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
- **Deterministic Hook Resolution:** Install and sync resolve portable hooks (`load-task`, `pre-plan`, `classify`, `label`, `decompose`, `post-plan`, `record-ledger`, `pre-deploy`, `post-deploy`) once from `~/.agent-hooks/`.
- **Atomic Manifest Ownership:** Project and global installs maintain `.agent-fabric-manifest.json`. Modified user files are preserved during sync unless `--force` is specified.
- **Task Continuity:** Supervisors must pass the recorded child continuation ID on later dispatches. A new session is allowed only for new resolving authority, an approved scope-identity change, or unavailable continuation. Reuse a finding's recorded diagnosis. Hook: `hooks/supervisor/record-ledger.py`. Tests: `python3 -m unittest discover -s tests -p 'test_supervisor_ledger.py'` and `TestSupervisorResumeHandleIsMandatory`.
- **Task Retrospective (opt-in):** One global `retrospective.enabled` switch. `hooks/retrospective/opencode.ts` is a thin OpenCode observation adapter handing versioned envelopes to `agf retrospective ingest` (stdin; success gating, dispatch, suppression, dedup all live in Go). Terminal triggers persist jobs and launch a detached `agf retrospective drain` (single-host lock, terminated daemon-less worker; server reconcile via the 2.x message API; `dispatch_endpoint: message`). Learning-origin sessions and descendants never capture. Runbook: `docs/runbooks/task-retrospective.md`. Spec: `docs/specs/task-retrospective.md`. Tests: `go test ./internal/retrospective/`, `npm ci/test --prefix hooks/retrospective`, `python3 -m unittest discover -s tests -p 'test_retrospective_release.py'`.
- **Read-only Preflight:** `python3 hooks/supervisor/support.py` (JSON stdin) reconciles host observations/owned Linux command identities, fingerprints declared actual inputs for verified receipt reuse, and validates compact handoffs. No dispatch, process control or new persistence. Contract: `docs/specs/agent-fabric.md`, Deterministic supervision support. Tests: `python3 -m unittest discover -s tests -p 'test_supervisor_support.py'`.
- **Supervisor Resolution:** Supervisors verify child blocker claims with capability probes and bounded recovery across tools, fixtures, workspaces, Dev state and scoped releases. Final blockers name the exact external-only action and failed probes; unavailable capability/credentials, product intent, explicit policy/scope boundaries and secret disclosure remain limits. Spec: `docs/specs/agent-fabric.md`.
- **Durable Task Authority:** Executable tasks authorize the complete in-scope flow without repeated confirmation, with proportional checkpoint/canary/rollback controls. Plan-only/report-only scope stops execution; release requires a named target, DoD, rollback and traceable authority. Spec: `docs/specs/agent-fabric.md`.
- **Budget Efficiency:** Use the least expensive adequate model, delegation count, and verification set; reuse sessions/evidence and escalate only after objective failure. Never reduce mandatory DoD gates for cost. Spec: `docs/specs/agent-fabric.md`.
- **Outcome-first QA:** Source-design fidelity is separate from screenshot regression; verify a representative page early. Repeated prerequisite recovery has a cross-role no-progress circuit breaker. Contract: `docs/specs/agent-fabric.md` (Outcome-first verification and bounded recovery).
- **Visible Progress:** Long checks require fixture preflight, receipt-driven updates and frozen-input independent review; private drafts are not public delivery. Contract: `docs/specs/agent-fabric.md` (Bounded episodes, waiting, and reduced delivery).
- **Crash-safe Resume:** Resume-critical checkpoints, candidates and receipts use a durable private root; temporary storage is scratch. Missing evidence is not PASS and does not reset budgets. Contract: `docs/specs/agent-fabric.md` (Bounded episodes, waiting, and reduced delivery).
- **Phase Closure:** Validate → independent review → scoped commit/integration → verified durable receipt → eligible owned cleanup; compact planning decisions, preserve unresolved evidence. Contract: `docs/specs/agent-fabric.md` (Phase closure and compact planning).
- **Discretionary QA:** Recommended, not mandatory; loop-supervisor decides run/skip/reuse/defer. QA-only execution blockers go upward without freezing implementation; unrun checks never become PASS. Setup changes belong to implementation roles under existing authority. Spec: `docs/specs/agent-fabric.md` (Discretionary QA Contract).
- **Acceptance & Recovery:** Current plan review and single projection owner remain required. Explicit user continuation may open a distinct bounded episode after soft exhaustion; preserve historical counters and hard limits. Wait for active children without repeated audits. Reduced delivery reuses the verified deployed revision and declares deferred/defective behavior. Spec: `docs/specs/agent-fabric.md`.

## Testing

- Unit tests in `internal/agent`, `internal/adapter`, `internal/manifest`, `internal/retrospective`, and `cmd/agent-fabric`.
- Golden fixture comparison in `internal/adapter/adapter_test.go` checks rendered output against `fixtures/`.
- Retrospective tests use recorded local mock HTTP and synthetic traces only; they never call the real Jev API or a real OpenCode server.
- The retrospective hooks package (`hooks/retrospective/`) owns the npm/TS tooling: `npm ci --prefix hooks/retrospective`, then `npm test --prefix hooks/retrospective` and `npm run typecheck --prefix hooks/retrospective` are deterministic and offline after ci. Recorded OpenCode event/envelope fixtures are shared with the Go replay tests in `fixtures/retrospective/opencode/`.
- Release/bootstrap fixture tests: `python3 -m unittest discover -s tests -p 'test_retrospective_release.py'` (local fake curl; no network, no real OpenCode server).
- Run `go test ./...` before committing.

## Secrets

Zero secrets, API keys, or provider tokens are tracked in this repository. Authentication is host-managed.

## Specs

Module and CLI behavioral contract lives in [`docs/specs/agent-fabric.md`](docs/specs/agent-fabric.md). Behavior change = update spec in the same commit as code.
