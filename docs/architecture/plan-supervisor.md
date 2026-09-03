# Plan Supervisor — Workflow Diagram

`agents/plan-supervisor.md` · profile: `supervisor` · mode: `primary` · isolation: `workspace`

The Plan Supervisor executes a pre-decomposed implementation plan phase-by-phase through bounded
task supervision. It owns phase ordering, evidence integrity, and bounded recovery.
Fresh-child arrows abbreviate the Delegation Packet contract defined normatively in
[`docs/specs/agent-fabric.md`](../specs/agent-fabric.md).
Direct user invocation is not a fresh-child handoff, so its intake packet is optional.

## Full Workflow

```mermaid
flowchart TD
    DIRECT([Direct user invocation\napproved plan · packet optional])
    FRESH([Fresh-child handoff])
    DIRECT --> LT
    FRESH -->|"intake Delegation Packet"| INTAKE
    INTAKE{"Intake packet complete and\nrequired locators resolve?"}
    INTAKE -- No --> BLOCKED_INTAKE([Return BLOCKED\n— exact context gap])
    INTAKE -- Yes --> LT

    subgraph "Load & Validate"
        LT["① load-task hook\nLoad explicit plan content or locator\noptionally via task system → build DAG"]
        PREVAL["pre-plan hook\nValidate source plan\n(if installed)"]
        DEC["decompose hook\nMaterialize phases if needed\n(if installed)"]
    end

    LT --> PREVAL --> DEC

    subgraph "Phase Selection Loop"
        SEL["② Select unblocked phase\nwith all predecessor evidence ready"]
        BRIEF["Write and validate\nself-locating phase packet"]
        DISPATCH["③ Dispatch loop-supervisor\nin fresh context with packet"]
    end

    DEC --> SEL --> BRIEF
    BRIEF -->|"validated phase packet"| DISPATCH

    subgraph "Evidence Verification"
        VERIFY["④ Verify phase evidence\n• Every DoD item PASS with\n  inspectable evidence\n• Record VCS revision"]
        RESULT{Phase result?}
    end

    DISPATCH --> VERIFY --> RESULT

    subgraph "Recovery"
        FAIL_CLASS["Classify failure\nenvironment · defect · spec-drift · flaky"]
        DIAG["Delegate bounded diagnostic\nwith validated recovery packet"]
        REBR["Refresh same phase packet\n+ diagnostic locator\nincrement attempt count"]
        EXHAUST{Recovery budget\nexhausted?}
        ESCALATE["Write PLAN_ESCALATION.md\n(attempts · root-cause · stable-rev\nrequired operator action)"]
    end

    RESULT -- "FAIL / BLOCKED" --> FAIL_CLASS
    FAIL_CLASS -->|"validated recovery packet"| DIAG
    DIAG --> REBR --> EXHAUST
    EXHAUST -- "No · refreshed phase packet" --> DISPATCH
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
| `record-ledger` | State transitions | Record structured macro-ledger event to host memory or fallback JSONL |

## Macro-Ledger & Phase Curation Firewall

The Plan Supervisor coordinates phase execution across the plan DAG and maintains a macro-ledger
at all phase transitions (`PENDING` -> `IN_PROGRESS` -> `DONE` or `BLOCKED`).

The Plan Supervisor acts as a curation firewall between phases:
- Outgoing phase packets contain strictly objective contracts, verified prerequisite outputs from the macro-ledger, and required acceptance gates.
- Subjective debates, prior phase debugging trails, or implementor rationalizations are strictly suppressed to prevent context pollution and cross-phase bias.

```json
{
  "tier": "macro",
  "plan_id": "<parent-task-or-plan-id>",
  "phase_id": "<phase-id>",
  "objective": "<phase-objective>",
  "summary": "<phase-status-summary>",
  "dependencies": ["<dep-phase-id>"],
  "status": "PENDING|IN_PROGRESS|DONE|BLOCKED",
  "revision": "<git-commit-hash>",
  "timestamp": "<iso-8601-utc>",
  "blockers": []
}
```

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
  "dod": [{"item": "...", "status": "PASS|FAIL|BLOCKED", "evidence": "authoritative locator or command"}],
  "required_gates": [{"command": "...", "status": "...", "evidence": "authoritative locator"}],
  "remaining_blockers": [],
  "changed_files": []
}
```
