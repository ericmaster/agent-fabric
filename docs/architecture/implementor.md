# Implementor — Workflow Diagram

`agents/implementor.md` · profile: `worker` · mode: `subagent` · isolation: `sandbox`

The Implementor makes bounded, targeted code changes for one atomic task. It is dispatched
by the loop-supervisor, hands off only to its supervisor, and never self-certifies.

## Full Workflow

```mermaid
flowchart TD
    START([Focused brief from loop-supervisor]) --> LT

    subgraph "Load Task"
        LT["load-task hook\nResolve atomic task from brief"]
        CHECK{"Brief has:\nobjective · non-goals · permitted paths\nobservable DoD · rollback boundary?"}
        GAP([Return gap to supervisor\n— do not invent design])
    end

    LT --> CHECK
    CHECK -- "Missing critical detail" --> GAP
    CHECK -- Complete --> INSPECT

    subgraph "Pre-implementation Inspection"
        INSPECT["Inspect destination:\nguidance · specs · relevant code\ntests · existing patterns"]
    end

    INSPECT --> IMPLEMENT

    subgraph "Implementation"
        IMPLEMENT["Make targeted, bounded changes\n• Absolute paths\n• Inspect before editing\n• Preserve unrelated user work\n• Align impl + tests + backing spec"]
        VERIFY["Run exact required checks\n+ strongest available relevant checks\n(substitute ≠ mandatory gate)"]
    end

    IMPLEMENT --> VERIFY

    subgraph "Handoff"
        DISCLOSE["Disclose to supervisor:\n• Changed files\n• Commands + results\n• Known risks\n• Exact verification evidence\n• Uncertain decisions\n• Blocked requirements"]
    end

    VERIFY --> DISCLOSE --> DONE([Return to loop-supervisor])

    style LT fill:#6366f1,color:#fff,stroke:none
```

## Hook Summary

| Hook | When | Purpose |
|---|---|---|
| `load-task` | Start | Resolve atomic task brief; validate task completeness before editing |

## Scope Constraints

| Allowed | Not Allowed |
|---|---|
| File edits within permitted paths | Committing or deploying changes |
| Running required checks | Authenticating with external services |
| Reporting evidence to supervisor | Starting long-lived services (unless explicitly granted) |
| Running strongest available additional checks | Redefining product architecture |
| | Silently broadening scope |
| | Handing off to anyone other than the supervisor |

## Authority Boundary

The implementor hands off **only to the loop-supervisor**. It does not dispatch code-reviewer or
qa-runner directly — the supervisor owns fresh-context review dispatch and the final verdict.
