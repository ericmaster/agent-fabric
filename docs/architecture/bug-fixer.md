# Bug Fixer — Workflow Diagram

`agents/bug-fixer.md` · profile: `supervisor` · mode: `primary` · isolation: `workspace`

The Bug Fixer interviews a reporter in plain language, persists one ticket per
defect, and autonomously dispatches the fix unless explicitly `report-only`. It
does not implement the fix itself or write later ticket updates.
Direct user invocation is not a fresh-child handoff, so its intake packet is optional.
Every fresh-child dispatch requires a validated self-locating Delegation Packet.
Later dispatches resume recorded child continuation IDs from the separate handoff
artifact; bug-fixer alone owns needs-plan projection and execution handoff.
The normative packet and fail-closed resolution contract lives only in
[`docs/specs/agent-fabric.md`](../specs/agent-fabric.md).

## Full Workflow

```mermaid
flowchart TD
    DIRECT([Direct user / plain-text bug\npacket optional])
    FRESH([Fresh-child handoff])
    DIRECT --> LT
    FRESH -->|"intake Delegation Packet"| INTAKE
    INTAKE{"Intake packet complete and\nrequired locators resolve?"}
    INTAKE -- No --> BLOCKED([Stop — exact context gap])
    INTAKE -- Yes --> LT

    LT["load-task hook (optional)\nResume from ticket locator"] --> BF
    BF["bug-fixer primary\nplain interview · summary card"]
    RR["report-reviewer\n(fresh context)"]
    HK{persist-ticket hook installed?}
    HOST["Host ticket + labels"]
    FILE["bugfix-tickets file default"]
    PLAN["planner"]
    ART["Explanation artifact"]
    DEC["decompose"]
    PSU["plan-supervisor"]
    LSU["loop-supervisor"]
    GATE{Standing execution authority?}

    BF -->|"optional validated review packet"| RR
    RR -->|"PASS or REVISE"| BF
    BF --> HK
    HK -- yes --> HOST
    HK -- no --> FILE
    FILE --> GATE
    HOST --> GATE
    GATE -- "report-only" --> STOP([Ticket persisted])
    GATE -- "exec-ready: explanation + validated packet" --> LSU
    GATE -- "needs-plan: validated planner packet" --> PLAN
    PLAN -->|"current plan-reviewer PASS"| ART
    ART -->|"validated autonomous projection"| DEC
    DEC --> PSU

    style LT fill:#6366f1,color:#fff,stroke:none
    style RR fill:#0ea5e9,color:#fff,stroke:none
    style DEC fill:#6366f1,color:#fff,stroke:none
    style PLAN fill:#a855f7,color:#fff,stroke:none
```

## Hook Summary

| Hook | Lifecycle Stage | Suggested Usage / Role | Default Behavior (No-op / Agent Decides) |
|---|---|---|---|
| `load-task` | Start / resume | Enrich or resolve a ticket locator | No-op — agent uses current user content |
| `persist-ticket` | After intake | Persist the portable ticket payload | File default under `bugfix-tickets/` with self-generated id |
| `label` | After persist | Tag the persisted ticket | No-op — continue without it |
| `decompose` | After validated explanation | Project validated children | Rendered instruction only; host owns execution |

## Lanes

| Lane | Trigger | Next dispatch |
|---|---|---|
| **exec-ready** | Single bounded fix, one component | `loop-supervisor` under standing authority |
| **needs-plan** | Multi-component or decomposable slices | `planner`, explanation, `decompose`, then `plan-supervisor` without repeated prompts |
