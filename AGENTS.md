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
skills/harness-review/     Model-invoked independent review and per-tool JSONL audit
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

CLI discovery: `agf --help`; `agf sync --help` and `agf hub install --help`
print flags/defaults without loading sources or writing state (exit 0).

| Command | What it does |
|---|---|
| `go test ./...` | Runs all unit and golden fixture tests |
| `go build ./cmd/agent-fabric` | Compiles the `agent-fabric` CLI binary |
| `scripts/release.sh <version> dist` | Builds cross-platform release archives, checksums, and manifest |

## Architecture at a glance

- **Secret guard:** `internal/guard` and `guard check` inspect shell syntax/read metadata with private denial audit; standalone, not installed enforcement. Contract: [`docs/specs/secret-guard.md`](docs/specs/secret-guard.md).
- **Canonical Definitions as SSOT:** Canonical agent workflows live exclusively in `agents/<id>.md` and contain full portable behavior, permissions, and declarative hook placeholders. Spec: `docs/specs/agent-fabric.md`.
- **Self-Locating Delegation:** Fresh-context handoffs use the fail-closed Delegation Packet contract defined normatively in `docs/specs/agent-fabric.md`; hooks only enrich or validate it.
- **Eval before implementation:** Pin measurable criteria before mutation; reconcile candidate-linked evidence before PASS, including direct atomic work. Contract: [`Pre-implementation evaluation baseline`](docs/specs/agent-fabric.md#pre-implementation-evaluation-baseline).
- **Harness-Isolated Rendering:** Adapters (`adapters/<target>.json` + `internal/adapter/`) map abstract profiles (`planner`, `worker`, `reviewer`, `supervisor`) to harness-specific models and tool syntax without contaminating canonical bodies.
- **Pi:** `extensions/pi` adds current-session Fabric role modes from global rendered agents; optional upstream `subagent` provides fresh-context delegation. Modes retain the active model and enforce exact tool lists, not OS isolation. Post-task review uses `harness-review`; setup/tests: `docs/runbooks/pi.md`.
- **OpenCode 2:** Native ordered permissions (`bash`→`shell`, `task`→`subagent`), `model#variant`, `hidden`; explicit `external_directory` guards survive tool allowlists (contract: `docs/specs/agent-fabric.md`). Portable hooks render as explicit body commands, not plugin frontmatter. Compatibility/rollout: `docs/runbooks/opencode2-supervision.md`.
- **Deterministic Hook Resolution:** Install and sync resolve portable hooks (`load-task`, `pre-plan`, `classify`, `label`, `decompose`, `post-plan`, `record-ledger`, `pre-deploy`, `post-deploy`) once from `~/.agent-hooks/`.
- **Atomic Manifest Ownership:** Project and global installs maintain `.agent-fabric-manifest.json`. Modified user files are preserved during sync unless `--force` is specified.
- **Task Continuity:** Supervisors must pass the recorded child continuation ID on later dispatches. A new session is allowed only for new resolving authority, an approved scope-identity change, or unavailable continuation. Reuse a finding's recorded diagnosis. Hook: `hooks/supervisor/record-ledger.py`. Tests: `python3 -m unittest discover -s tests -p 'test_supervisor_ledger.py'` and `TestSupervisorResumeHandleIsMandatory`.
- **Instincts (advisory lessons):** Signed reusable lessons may carry `confidence` ∈ [0.3, 0.9] and `applicability` `project` (default) | `global`. Weighted advisory only — never DoD/permission authority. Contract: `docs/specs/task-retrospective.md` (Reusable lessons / Instincts).
- **Harness review:** After main-task completion, load `skills/harness-review/SKILL.md` to evaluate repeated tool errors, context searches without progress, ambiguous/misleading context or stalls. Retain evidence during execution; only an explicit early-review request overrides the completion gate. A visible supervisor uses a persisted read-only worker and independently probes blocker claims; accessible unfinished review work stays pending. Carry standing maintenance authority into review launches; read-only applies to investigation workers, not authorized supervisor correction. Correct proven causes with tests and independent patch review. Install only the short native-context activation pointer; no observers, GLiDE or extra service. OpenCode history reads and root launches use the verified `skills/harness-review/references/opencode.md` procedure. `task-review` is a deprecated redirect. Contract: `docs/specs/task-retrospective.md`.
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

- **Evidence:** JSONL entries are agent reports, not automatic proof of success. Record native session IDs, actual checks, patch/revision and outcome; preserve failures and history. Route harness-review roles at skill step 0 before launch; review/correction leaves never start recursive retrospectives. Legacy retrospective CLI/data remain compatibility utilities, not part of the skill's delivery path.

## Testing

- Unit tests in `internal/agent`, `internal/adapter`, `internal/manifest`, `internal/retrospective`, and `cmd/agent-fabric`.
- Golden fixture comparison in `internal/adapter/adapter_test.go` checks rendered output against `fixtures/`.
- Skill/audit tests: `python3 -m unittest discover -s tests -p 'test_harness_review_audit.py'`. No provider/API key required.
- Legacy retrospective tests use local mocks and preserve historical failure/permission/provenance guards; no live provider call is implied by passing them.
- Release/bootstrap fixture tests: `python3 -m unittest discover -s tests -p 'test_retrospective_release.py'` (local fake curl; no network, no real OpenCode server).
- Run `go test ./...` before committing.

## Secrets

Zero secrets, API keys, or provider tokens are tracked in this repository. Authentication is host-managed.

## Specs

Module and CLI behavioral contract lives in [`docs/specs/agent-fabric.md`](docs/specs/agent-fabric.md). Behavior change = update spec in the same commit as code.
