# Agent Fabric — Agent Guide

Agent Fabric provides portable, provider-neutral agent definitions and adapters that compile
canonical workflows into native configurations for OpenCode, Kilo, Antigravity CLI, Codex, Claude Code, and Pi.

> [!IMPORTANT]
> **AGENTS.md CRITICAL RULES:**
> **No Fluff:** Minimum characters. Concise but 100% complete.
> **Documentation:** Before documentation changes, apply `context-hygiene` and read
> [filing, current-state, release-history and ADR rules](docs/docs-organization-blueprint.md#1-the-clean-divide-versioned-wiki-vs-scratch-space).
> Update authorized plans in Projektor and add one short decision paragraph on each affected issue; apply context hygiene rather than creating decision artifacts.
> **Live Sync:** Keep this file updated with relevant code changes in the same commit.
> **Human workflow reference:** [Agent workflow](https://artifacts.nimblersoft.com/project/agent-fabric/artifact/agent-fabric-architecture-lifecycle)
> `.artifacts-manager/agent-fabric-architecture-lifecycle.html` is the single human overview;
> canonical agents and specs define behavior, and Projektor owns plans and task decisions.
> Update the artifact and manifest metadata in the same change as routing, role, gate,
> recovery, delivery or post-task-review changes; verify against canonical sources.
> Keep it current-state only; other human entry points link here.
> **Ownership:** This file owns repository working guidance; [docs/index.md](docs/index.md)
> locates domain and module owners. Do not add parallel MAINTENANCE.md / ARCHITECTURE.md guides.

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
internal/router/         Advisory router: easy → implementor; complex → planner (`agent-fabric route`)
docs/                    Versioned wiki (docs-organization-blueprint.md, specs/, adr/, runbooks/)
fixtures/                Golden test fixtures for adapter rendering (+ retrospective/opencode recorded events/envelopes)
scripts/                 Release packaging and bootstrap tooling
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

Canonical `agents/<id>.md` bodies carry complete portable workflows; adapters render
native configurations. Preserve standalone role instructions when the source repository
is unavailable to installed agents. This repository guide routes maintainers to contracts;
it does not replace exported role behavior.

Read the relevant contract before changing or applying its behavior:

- **Definitions, adapters, permissions and install ownership:** [Core schema, adapters and manifest ownership](docs/specs/agent-fabric.md#core-schema-adapters-and-manifest-ownership).
- **Hooks, Codex rendering and releases:** [Hook resolution and packaging](docs/specs/agent-fabric.md#hook-resolution-codex-adapter-and-release-packaging).
- **Pi / OpenCode host setup and compatibility:** [Pi runbook](docs/runbooks/pi.md), including isolated version-pinned test fixtures; [OpenCode 2 runbook](docs/runbooks/opencode2-supervision.md).
- **Delegation / implementation:** Before dispatch or mutation, read [packet validation](docs/specs/delegation-and-evaluation.md#fresh-context-delegation-contract) and [evaluation baseline and acceptance](docs/specs/delegation-and-evaluation.md#pre-implementation-evaluation-baseline).
- **Routing, implementation/execution phase ownership, authority, recovery and budgets:** [Autonomous execution](docs/specs/supervision-and-delivery.md#autonomous-execution-contract); CLI recommendations: [difficulty router](docs/specs/difficulty-router.md).
- **Planning / unfinished work:** [Planning readiness](docs/specs/supervision-and-delivery.md#planning-readiness), [plan amendment](docs/specs/supervision-and-delivery.md#diagnosis-driven-plan-amendment), and [unfinished-stop status](docs/specs/supervision-and-delivery.md#unfinished-stop-status).
- **Progress, durable resume and delivery:** [Bounded episodes](docs/specs/supervision-and-delivery.md#bounded-episodes-waiting-and-reduced-delivery), [checkout closure](docs/specs/supervision-and-delivery.md#delivery-ownership-and-checkout-closure), [async orchestration](docs/specs/supervision-and-delivery.md#native-asynchronous-orchestration), and [phase closure](docs/specs/supervision-and-delivery.md#phase-closure-and-compact-planning).
- **Preflight / ledger / QA:** [Read-only support](docs/specs/supervision-ledger-and-qa.md#deterministic-supervision-support), [continuity and compact checkpoints](docs/specs/supervision-ledger-and-qa.md#continuity-and-checkpoints), [outcome-first verification](docs/specs/supervision-ledger-and-qa.md#outcome-first-verification-and-bounded-recovery), and [discretionary QA](docs/specs/supervision-ledger-and-qa.md#discretionary-qa-contract).
- **Secret guard changes or installation:** [Guard coverage and limits](docs/specs/secret-guard.md); do not infer enforcement from installation.
- **Task completion:** Evaluate and communicate the [harness-review decision](docs/specs/task-retrospective.md#decision-ownership-and-closure) after [the completion gate](docs/specs/task-retrospective.md#entry-point). On activation, load [the executable skill](skills/harness-review/SKILL.md) and route the assignment at step 0 before launch.
- **Review audit / advisory lessons:** [Audit evidence](docs/specs/task-retrospective.md#audit), [runtime and legacy compatibility](docs/specs/task-retrospective.md#runtime-boundary-and-compatibility), and [Instincts](docs/specs/task-retrospective.md#reusable-lessons--instincts).

## Testing

- Unit tests in `internal/agent`, `internal/adapter`, `internal/manifest`, `internal/retrospective`, `internal/router`, and `cmd/agent-fabric`.
- Golden fixture comparison in `internal/adapter/adapter_test.go` checks rendered output against `fixtures/`.
- Skill/audit tests: `python3 -m unittest discover -s tests -p 'test_harness_review_audit.py'`. No provider/API key required.
- Legacy retrospective tests use local mocks and preserve historical failure/permission/provenance guards; no live provider call is implied by passing them.
- Release/bootstrap fixture tests: `python3 -m unittest discover -s tests -p 'test_retrospective_release.py'` (local fake curl; no network, no real OpenCode server).
- Supervisor hooks: `python3 -m unittest discover -s tests -p 'test_supervisor_*.py'`; continuation also has the Go guard `TestSupervisorResumeHandleIsMandatory`.
- Run `go test ./...` before committing.

## Secrets

Zero secrets, API keys, or provider tokens are tracked in this repository. Authentication is host-managed.

## Specs

Behavior change = update the owning spec above in the same commit as code.
