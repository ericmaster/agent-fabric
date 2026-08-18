# Planner — Workflow Diagram

`agents/planner.md` · profile: `planner` · mode: `primary` · isolation: `sandbox`

The Planner authors grounded, independently reviewed implementation plans as executable vertical
slices. It never executes phases or treats a proposal as approval.

## Full Workflow

```mermaid
flowchart TD
    START([User / Task Seed]) --> LT

    subgraph "Initial Understanding"
        LT["① load-task hook\nResolve seed from request,\nexplicit file, or task-system item"]
        CL["② classify hook\nResolve destination from brief\nand repository ownership evidence"]
        READ["③ Read destination guidance\nSpecs · ADRs · tests · patterns"]
        MAP["④ Map affected surfaces\nEntry points · contracts · data flow\npersistence · UI/API · rollback seams"]
        GAPS["⑤ Delegate bounded discovery\nor design questions (if needed)"]
        BRIEF["⑥ State objective, non-goals,\ncurrent behavior, destination,\nconstraints, and decision-critical gaps"]
    end

    LT --> CL --> READ --> MAP --> GAPS --> BRIEF

    subgraph "Design"
        DESIGN["Design smallest coherent approach\n• Vertical slices (not horizontal layers)\n• Prefactor only when needed for safety\n• Minimal, acyclic dependencies\n• Each phase independently delegable\n• Visual acceptance criteria for UI"]
    end

    BRIEF --> DESIGN

    subgraph "Pre-Plan Hook (optional)"
        PP["pre-plan hook\nValidation / enrichment rules\n(skip if not installed)"]
    end

    DESIGN --> PP

    subgraph "Independent Review"
        REVIEWER["Spawn plan-reviewer\nin fresh context\n(required for multi-phase plans)"]
        VERDICT{Reviewer verdict?}
        REVISE["Incorporate supported\ncorrections · rebuild\naffected phase boundaries"]
    end

    PP --> REVIEWER --> VERDICT
    VERDICT -- REVISE --> REVISE --> REVIEWER
    VERDICT -- PASS --> PROPOSE

    subgraph "Proposal Boundary"
        PROPOSE["Write canonical plan body\n(Phase-block form)"]
        LBL["label hook\nApply labels / state"]
        DEC["decompose hook\nProject child tasks into\ntask-system (if installed)"]
        PPOST["post-plan hook\nPublish / notify (if installed)"]
    end

    PROPOSE --> LBL --> DEC --> PPOST --> DONE([Plan ready for approval])

    style LT fill:#6366f1,color:#fff,stroke:none
    style CL fill:#6366f1,color:#fff,stroke:none
    style PP fill:#a855f7,color:#fff,stroke:none
    style REVIEWER fill:#0ea5e9,color:#fff,stroke:none
    style LBL fill:#6366f1,color:#fff,stroke:none
    style DEC fill:#6366f1,color:#fff,stroke:none
    style PPOST fill:#6366f1,color:#fff,stroke:none
```

## Hook Summary

| Hook | When | Installed behaviour | Not installed |
|---|---|---|---|
| `load-task` | Step 1 | Custom task resolution / enrichment | Agent resolves seed from context |
| `classify` | Step 2 | Route to project · team · service catalog | Agent infers destination from evidence |
| `pre-plan` | Before proposal | Validation gate (blocks on reject) | Continue without validation |
| `label` | Proposal boundary | Apply task-system labels | No-op |
| `decompose` | Proposal boundary | Project phases as child tasks | No-op |
| `post-plan` | After proposal | Publish · notify · trigger downstream | No-op |

## Operating Modes

| Mode | Trigger | Guard |
|---|---|---|
| **Author** | New goal, brief, file, or task seed | — |
| **Refine** | Eligible draft plan exists | Must still be a draft; not approved, projected, or blocked by children |
| **Approval / projection** | Explicit operator authorization | Requires stored body + auditable receipt |
