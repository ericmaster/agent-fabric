# Code Reviewer — Workflow Diagram

`agents/code-reviewer.md` · profile: `reviewer` · mode: `subagent` · isolation: `sandbox`

The Code Reviewer is an adversarial, read-only gate dispatched by the loop-supervisor in a
fresh context. It cannot edit files. It returns a single structured JSON verdict.
Its intake follows the fail-closed packet contract defined normatively in
[`docs/specs/agent-fabric.md`](../specs/agent-fabric.md).

## Full Workflow

```mermaid
flowchart TD
    START([Validated packet + diff from loop-supervisor]) --> LT

    subgraph "Load Task"
        LT["load-task hook\nEnrich or validate packet only\n(context gap → role-equivalent failure)"]
    end

    LT --> STATIC

    subgraph "Static Analysis First"
        STATIC["Run safe static checks\n(syntax · imports · types · linting)"]
        STATIC_FAIL{Static gates\npass?}
        REJECT_STATIC([Immediate REJECT finding\n— no architectural review\nfor code that cannot compile])
    end

    STATIC --> STATIC_FAIL
    STATIC_FAIL -- No --> REJECT_STATIC --> OUTPUT
    STATIC_FAIL -- Yes --> SEMANTIC

    subgraph "Semantic Review"
        SEMANTIC["Review diff against DoD\nand supplied specification"]

        CHECK_LIST["Check for:\n• Missing code-level evidence\n  (unit test coverage of touched paths)\n• Hollow tests (mutation check)\n• Security boundary violations\n  (untrusted input → exec sinks)\n• Unsafe file paths & type escapes\n• Shotgun / duplicated edits\n• Scope drift beyond supplied DoD\n• Swallowed errors\n• Note: Dynamic QA/screenshots deferred to qa-runner"]

        MUTATION["Apply mental mutation test:\nWould reverting the behaviour\nleave new tests green?\nIf yes → tests don't prove the change"]
    end

    SEMANTIC --> CHECK_LIST --> MUTATION --> SEVERITY

    subgraph "Severity Ordering"
        SEVERITY["Order findings by severity\n(critical → high → medium → low)\nwith file/symbol references\nand verification gaps"]
    end

    SEVERITY --> VERDICT{Any material\ndefect?}

    VERDICT -- Yes --> REJECT([REJECT])
    VERDICT -- No --> ACCEPT([ACCEPT])

    REJECT --> OUTPUT
    ACCEPT --> OUTPUT

    subgraph "Output"
        OUTPUT["Return exactly one JSON object"]
    end

    OUTPUT --> DONE([Return to loop-supervisor])

    style LT fill:#6366f1,color:#fff,stroke:none
```

## Hook Summary

| Hook | When | Purpose |
|---|---|---|
| `load-task` | Start | Enrich or validate supplied packet context; never reconstruct a known locator |

## Output Contract

```json
{
  "verdict": "ACCEPT|REJECT",
  "contract_adherence": {
    "is_aligned_with_dod": true,
    "missing_requirements": []
  },
  "static_analysis": {
    "compilation_status": "PASS|FAIL|NOT_AVAILABLE",
    "commands_run": [],
    "compiler_errors": []
  },
  "findings": [
    {
      "severity": "critical|high|medium|low",
      "evidence": "path or symbol",
      "required_change": ""
    }
  ]
}
```

## Hard Reject Triggers

| Trigger | Severity |
|---|---|
| Syntax / import / type failure | `critical` — immediate REJECT, no further review |
| Untrusted input flowing into execution sink | `critical` |
| Unsafe file paths | `critical` |
| Hollow test (mutation would leave green) | `high` |
| Swallowed error | `high` |
| Scope drift beyond supplied DoD | `high` |
| Unsupported scope expansion | Reject as finding |

> [!NOTE]
> **Dynamic Gate Separation**: The Code Reviewer evaluates code diffs, types, and unit test validity. It must never reject tasks for missing runtime execution, persistence/payload checks, visual screenshots, or deployment validation; dynamic gates are deferred exclusively to `qa-runner`.
