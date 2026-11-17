# Agent Fabric — Documentation

Wiki index for this project. Agent working context lives in
[AGENTS.md](../AGENTS.md); domain terminology in [CONTEXT.md](../CONTEXT.md).

| Folder | Contains | Answers |
|---|---|---|
| [`architecture/`](architecture/) | Links to the human workflow reference and canonical roles | *Where do I learn the agent workflow?* |
| [`adr/`](adr/) | Architectural Decision Records | *Why is it built this way?* |
| [`specs/`](specs/) | Module contracts and feature specs | *What must this module do?* |
| [`runbooks/`](runbooks/) | Exact command sequences | *What do I run to do X?* |
| [`research/`](research/) | Takeaways and reports (full sources in the scratch space) | *What did we learn?* |

Filing rules and frontmatter schemas are SSOT in
[`docs-organization-blueprint.md`](docs-organization-blueprint.md) — consult §3 when a
document's home is ambiguous, and apply the frontmatter from §4.

## Architecture

- [Agent workflow](https://artifacts.nimblersoft.com/project/agent-fabric/artifact/agent-fabric-architecture-lifecycle) — Single human overview: routing, planning, implementation, recovery, delivery and post-task review.
- [`../agents/`](../agents/) — Executable role definitions; normative details remain in the specs below.

## Decisions

<!-- TODO: one line per ADR, newest last -->

## Specs

- [`specs/secret-guard.md`](specs/secret-guard.md) — Standalone shell/read checker, limits and private denial audit.
- [`specs/agent-fabric.md`](specs/agent-fabric.md) — Core CLI, portable agent schema, multi-harness adapter rendering, manifest tracking, hook resolution, and release packaging.
- [`specs/delegation-and-evaluation.md`](specs/delegation-and-evaluation.md) — Fresh-context Delegation Packet contract, pre-implementation evaluation baseline, and external pre-review evidence.
- [`specs/supervision-and-delivery.md`](specs/supervision-and-delivery.md) — Autonomous execution authority, bounded episodes, delivery ownership, native asynchronous orchestration, phase closure, and bug intake orchestrator.
- [`specs/supervision-ledger-and-qa.md`](specs/supervision-ledger-and-qa.md) — Deterministic supervision support (`support.py`), discretionary QA and startup contracts, and supervisor ledger hook (`record-ledger.py`), continuity, and curation firewall.
- [`specs/task-retrospective.md`](specs/task-retrospective.md) — Harness review: supervisor–worker investigation, verified blockers, scoped correction and one audit per engine.
  Specialist detail: [Harness review](https://artifacts.nimblersoft.com/project/agent-fabric/artifact/harness-review-guide), linked from the human workflow reference.
- [`specs/retrospective-monitor.md`](specs/retrospective-monitor.md) — Opt-in native session observer (`monitor.py`) and spool retention contract.

## Runbooks

- [`runbooks/pi.md`](runbooks/pi.md) — Pi subagent runtime, Fabric agents/hooks, and PiChamber setup.
- [`runbooks/opencode2-supervision.md`](runbooks/opencode2-supervision.md) — OpenCode 2 permission compilation, continuation semantics, and rollout checks.
- [`runbooks/opencode-secret-guard.md`](runbooks/opencode-secret-guard.md) — Opt-in OpenCode secret-guard install. Contract: [`specs/secret-guard.md`](specs/secret-guard.md).
- [`runbooks/task-retrospective.md`](runbooks/task-retrospective.md) — Activate harness review, migrate its deprecated name, inspect audits and retire old hooks.
- [`runbooks/retrospective-monitor.md`](runbooks/retrospective-monitor.md) — Run, verify, and retire the opt-in native session monitor.
