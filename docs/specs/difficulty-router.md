---
title: "Spec: advisory difficulty router"
type: spec
status: active
covers: internal/router/, cmd/agent-fabric/route.go
last_checked: 2026-11-06
---

# Advisory Difficulty Router

`agent-fabric route [flags] TEXT|-` recommends top-level owning entrypoints;
it never spawns agents, monitors tests or retries work. `pipeline` is
`["implementor"]` for easy and `["planner"]` for complex. Planner retains its
existing handoff contract; implementor owns tests, remediation and independent
review. A recommendation is not an exemption from review or safety gates.

## Heuristic and overrides

Each signal adds one point; score >= 2 recommends complex:
- More than 120 whitespace-separated words.
- More than two expected files (`--files`, zero means unknown).
- Each distinct keyword prefix in `internal/router/router.go`, capped at two.
  Match lowercase word prefixes, not arbitrary substrings (`author` is not
  authentication). Letter/digit/hyphen tokens preserve `multi-step`.

Empty text recommends complex. `--difficulty auto|easy|complex` retains explicit
override precedence. `--escalate REASON` explicitly recommends complex without
dispatch; escalating complex is a no-op. Ordinary red tests do not automatically
escalate: implementor owns ordinary diagnosis and repairs.

## CLI contract

Text output is one line; `--json` opts into the existing `difficulty`, `pipeline`,
`score`, `reasons` shape. Reasons remain advisory. Flags precede text; `-` reads
at most 1 MiB from stdin. Reject negative file counts, invalid difficulty,
whitespace-only escalation reasons and escalation combined with an easy override.
Help exits successfully without routing or writing state.

Regression gates: router/route/help tests. Lean execution and checkpoint continuity
are governed by [supervision-and-delivery.md](supervision-and-delivery.md#phase-ownership)
and [supervision-ledger-and-qa.md](supervision-ledger-and-qa.md).
