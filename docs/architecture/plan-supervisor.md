# Plan Supervisor — Workflow Diagram

`agents/plan-supervisor.md` · profile: `supervisor` · mode: `primary` · isolation: `workspace`

The Plan Supervisor executes a pre-decomposed implementation plan phase-by-phase through bounded
task supervision. It owns phase ordering, evidence integrity, and bounded recovery.

## Full Workflow

```mermaid
flowchart TD
    START([Approved plan / task-system parent]) --> LT

    subgraph "Load & Validate"
        LT["① load-task hook\nLoad pre-decomposed plan or\ntask-system parent → build DAG"]
        PREVAL["pre-plan hook\nValidate source plan\n(if installed)"]
        DEC["decompose hook\nMaterialize phases if needed\n(if installed)"]
    end

    LT --> PREVAL --> DEC

    subgraph "Phase Selection Loop"
        SEL["② Select unblocked phase\nwith all predecessor evidence ready"]
        BRIEF["Write isolated phase brief\n(objective · scope · artifact paths\nDoD · allowed files · gates · commit owner)"]
        DISPATCH["③ Dispatch loop-supervisor\nin fresh context"]
    end

    DEC --> SEL --> BRIEF --> DISPATCH

    subgraph "Evidence Verification"
        VERIFY["④ Verify phase evidence\n• Every DoD item PASS with\n  inspectable evidence\n• Record VCS revision"]
        RESULT{Phase result?}
    end

    DISPATCH --> VERIFY --> RESULT

    subgraph "Recovery"
        FAIL_CLASS["Classify failure\nenvironment · defect · spec-drift · flaky"]
        DIAG["Delegate bounded\ndiagnostic task"]
        REBR["Re-brief same phase\n+ diagnostic artifact\nincrement attempt count"]
        EXHAUST{Recovery budget\nexhausted?}
        ESCALATE["Write PLAN_ESCALATION.md\n(attempts · root-cause · stable-rev\nrequired operator action)"]
    end

    RESULT -- "FAIL / BLOCKED" --> FAIL_CLASS --> DIAG --> REBR --> EXHAUST
    EXHAUST -- No --> DISPATCH
    EXHAUST -- Yes --> ESCALATE --> DONE_FAIL([Plan blocked / escalated])

    subgraph "Phase Completion"
        LBL["⑤ label hook\nRecord phase state in task-system\n(if installed)"]
        MORE{All phases\ncomplete?}
    end

    RESULT -- PASS --> LBL --> MORE
    MORE -- "No — next eligible phase" --> SEL
    MORE -- "Yes (all phases PASS)" --> DONE_OK([Plan complete])

    style LT fill:#6366f1,color:#fff,stroke:none
    style PREVAL fill:#a855f7,color:#fff,stroke:none
    style DEC fill:#6366f1,color:#fff,stroke:none
    style LBL fill:#6366f1,color:#fff,stroke:none
```

## Hook Summary

| Hook | Step | Purpose |
|---|---|---|
| `load-task` | 1 | Resolve plan source; build phase DAG |
| `pre-plan` | Pre-dispatch | Validate source plan before any mutation |
| `decompose` | Pre-dispatch | Materialize phases into task-system (if needed) |
| `label` | After each PASS | Record phase state in task-system |

## Autonomous Boundaries

| Condition | Behaviour |
|---|---|
| Successful non-final phase | Continue immediately — no human pause |
| Phase explicitly `operator-required` | Pause and wait for human action |
| Recovery budget exhausted | Produce `PLAN_ESCALATION.md`; stop |
| Irreversible environment / authority failure | Stop; preserve evidence |

## Evidence Contract (per phase)

```json
{
  "status": "PASS|FAIL|BLOCKED",
  "dod": [{"item": "...", "status": "PASS|FAIL|BLOCKED", "evidence": "path or command"}],
  "required_gates": [{"command": "...", "status": "...", "evidence": "..."}],
  "remaining_blockers": [],
  "changed_files": []
}
```
