# Implementor — Workflow Diagram

`agents/implementor.md` · profile: `worker` · mode: `subagent` · isolation: `sandbox`

The Implementor makes bounded, targeted code changes for one atomic task. It is dispatched
by the loop-supervisor, hands off only to its supervisor, and never self-certifies.
Its intake follows the fail-closed packet contract defined normatively in
[`docs/specs/agent-fabric.md`](../specs/agent-fabric.md); the diagram assumes required
inputs resolved before normal repository inspection.

## Full Workflow

```mermaid
flowchart TD
    START([Validated Delegation Packet]) --> LT

    subgraph "Load Task"
        LT["load-task hook\nEnrich or validate packet only"]
        CHECK{"Required inline inputs and\npacket locators resolve?"}
        GAP([Return BLOCKED\nwith exact context gap])
    end

    LT --> CHECK
    CHECK -- "Missing critical detail" --> GAP
    CHECK -- Complete --> INSPECT

    subgraph "Pre-implementation Inspection"
        INSPECT["Inspect destination:\nguidance · specs · relevant code\ntests · existing patterns"]
    end

    INSPECT --> IMPLEMENT

    subgraph "Implementation"
        IMPLEMENT["Make targeted, bounded changes\n• Whole-system lifecycle audit\n• Upfront defensive concurrency & retries\n• Packet-declared roots\n• Inspect before editing\n• Align impl + tests + backing spec"]
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
| `load-task` | Start | Enrich or validate supplied packet context; never reconstruct a known locator |

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
