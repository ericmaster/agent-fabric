# Agent Fabric — Documentation

Wiki index for this project. Agent working context lives in
[AGENTS.md](../AGENTS.md); domain terminology in [CONTEXT.md](../CONTEXT.md).

| Folder | Contains | Answers |
|---|---|---|
| [`architecture/`](architecture/) | Agent workflow diagrams, system overview | *How does each agent work? How do they interconnect?* |
| [`adr/`](adr/) | Architectural Decision Records | *Why is it built this way?* |
| [`specs/`](specs/) | Module contracts and feature specs | *What must this module do?* |
| [`runbooks/`](runbooks/) | Exact command sequences | *What do I run to do X?* |
| [`research/`](research/) | Takeaways and reports (full sources in the scratch space) | *What did we learn?* |

Filing rules and frontmatter schemas are SSOT in
[`docs-organization-blueprint.md`](docs-organization-blueprint.md) — consult §3 when a
document's home is ambiguous, and apply the frontmatter from §4.

## Architecture

- [`architecture/system-overview.md`](architecture/system-overview.md) — Full system diagram: all agents, hooks, and delegation paths
- [`architecture/planner.md`](architecture/planner.md) — Planner workflow and hook map
- [`architecture/plan-reviewer.md`](architecture/plan-reviewer.md) — Plan Reviewer review rubric and Planner↔Reviewer sequence
- [`architecture/plan-supervisor.md`](architecture/plan-supervisor.md) — Plan Supervisor phase-execution state machine
- [`architecture/loop-supervisor.md`](architecture/loop-supervisor.md) — Loop Supervisor atomic task control loop and delegation model
- [`architecture/implementor.md`](architecture/implementor.md) — Implementor bounded-change workflow
- [`architecture/code-reviewer.md`](architecture/code-reviewer.md) — Code Reviewer static + semantic review protocol
- [`architecture/qa-runner.md`](architecture/qa-runner.md) — QA Runner verification sequence and anti-cheat checks
- [`architecture/expert-debugger.md`](architecture/expert-debugger.md) — Expert Debugger hypothesis-elimination and remediation brief

## Decisions

<!-- TODO: one line per ADR, newest last -->

## Specs

- [`specs/agent-fabric.md`](specs/agent-fabric.md) — Portable agent schema, multi-harness adapter rendering, manifest tracking, and disposable release gate verification.

## Runbooks

<!-- TODO: one line per runbook -->
