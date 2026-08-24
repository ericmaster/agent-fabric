# Agent Fabric — System Architecture

This diagram shows how all eight built-in agents collaborate in the full planning → execution
lifecycle, including hook event fire points and delegation relationships.

## Full System Diagram

```mermaid
flowchart TD
    USER([User / Operator]) --> PLANNER

    subgraph "Planning Pipeline"
        PLANNER["**Planner**\nprofile: planner · primary"]
        H_LT1["🪝 load-task"]
        H_PP1["🪝 pre-plan"]
        PREV["**Plan Reviewer**\nprofile: reviewer · subagent\n(fresh context)"]
        H_POST1["🪝 post-plan"]
    end

    PLANNER --> H_LT1 --> H_PP1 --> PREV
    PREV -- PASS --> H_POST1

    H_POST1 --> APPROVED([Approved plan])

    subgraph "Plan Supervision Pipeline"
        PSUP["**Plan Supervisor**\nprofile: supervisor · primary"]
        H_LT2["🪝 load-task"]
        H_PP2["🪝 pre-plan"]
        H_DEC2["🪝 decompose"]
        H_LBL2["🪝 label (per phase)"]
    end

    APPROVED --> PSUP
    PSUP --> H_LT2 --> H_PP2 --> H_DEC2

    subgraph "Atomic Execution Loop (per phase)"
        LSUP["**Loop Supervisor**\nprofile: supervisor · primary"]
        H_LT3["🪝 load-task"]
        IMPL["**Implementor**\nprofile: worker · subagent"]
        IMPL_H_LT["🪝 load-task"]
        CREV["**Code Reviewer**\nprofile: reviewer · subagent\n(fresh context)"]
        CREV_H_LT["🪝 load-task"]
        QAR["**QA Runner**\nprofile: qa · subagent"]
    end

    H_DEC2 --> LSUP
    LSUP --> H_LT3 --> IMPL
    IMPL --> IMPL_H_LT
    IMPL --> CREV
    CREV --> CREV_H_LT
    CREV --> QAR
    QAR --> LSUP

    subgraph "Recovery (on FAIL / BLOCKED)"
        DBG["**Expert Debugger**\nprofile: readonly · subagent"]
    end

    LSUP -.->|"failure evidence"| DBG
    DBG -.->|"remediation brief"| LSUP

    LSUP --> H_LBL2 --> PHASE_DONE{All phases\ncomplete?}
    PHASE_DONE -- "next phase" --> H_DEC2
    PHASE_DONE -- "all done" --> COMPLETE([Plan complete])

    style H_LT1 fill:#6366f1,color:#fff,stroke:none
    style H_PP1 fill:#a855f7,color:#fff,stroke:none
    style H_POST1 fill:#6366f1,color:#fff,stroke:none
    style H_LT2 fill:#6366f1,color:#fff,stroke:none
    style H_PP2 fill:#a855f7,color:#fff,stroke:none
    style H_DEC2 fill:#6366f1,color:#fff,stroke:none
    style H_LBL2 fill:#6366f1,color:#fff,stroke:none
    style H_LT3 fill:#6366f1,color:#fff,stroke:none
    style IMPL_H_LT fill:#6366f1,color:#fff,stroke:none
    style CREV_H_LT fill:#6366f1,color:#fff,stroke:none
```

## Agent Role Summary

| Agent | Profile | Mode | Isolation | Hooks |
|---|---|---|---|---|
| [Planner](planner.md) | `planner` | primary | sandbox | load-task · pre-plan · post-plan |
| [Plan Reviewer](plan-reviewer.md) | `reviewer` | subagent | sandbox | pre-plan |
| [Plan Supervisor](plan-supervisor.md) | `supervisor` | primary | workspace | load-task · pre-plan · label · decompose |
| [Loop Supervisor](loop-supervisor.md) | `supervisor` | all | workspace | load-task |
| [Implementor](implementor.md) | `worker` | subagent | sandbox | load-task |
| [Code Reviewer](code-reviewer.md) | `reviewer` | subagent | sandbox | load-task |
| [QA Runner](qa-runner.md) | `qa` | subagent | sandbox | — |
| [Expert Debugger](expert-debugger.md) | `readonly` | subagent | sandbox | — |

## Hook Event Reference

| Event | Registered by | Typical purpose |
|---|---|---|
| `load-task` | Planner · Plan Supervisor · Loop Supervisor · Implementor · Code Reviewer | Resolve task / brief from host task-system or context |
| `pre-plan` | Planner · Plan Supervisor · Plan Reviewer | Validation gate and schema/constraint loading before planning or review begins |
| `classify` | — (reserved; no built-in agent registers it) | Route to destination / select next unblocked phase |
| `label` | Plan Supervisor | Apply task-system labels or state transitions |
| `decompose` | Plan Supervisor | Project child phases into task-system |
| `post-plan` | Planner | Signal completion / publish / notify |

Hooks are resolved once at install/sync time from `~/.agent-hooks/`. Markdown instructions
take precedence over executable scripts. When no hook is installed, agents continue without it.
