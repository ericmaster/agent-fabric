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
        H_CL["🪝 classify"]
        H_PP1["🪝 pre-plan"]
        PREV["**Plan Reviewer**\nprofile: reviewer · subagent\n(fresh context)"]
        H_LBL["🪝 label"]
        H_DEC["🪝 decompose"]
        H_POST1["🪝 post-plan"]
    end

    PLANNER --> H_LT1 --> H_CL --> H_PP1 --> PREV
    PREV -- PASS --> H_LBL --> H_DEC --> H_POST1

    H_POST1 --> APPROVED([Approved plan])

    subgraph "Plan Supervision Pipeline"
        PSUP["**Plan Supervisor**\nprofile: supervisor · primary"]
        H_LT2["🪝 load-task"]
        H_CL2["🪝 classify"]
        H_PP2["🪝 pre-plan"]
        H_DEC2["🪝 decompose"]
        H_LBL2["🪝 label (per phase)"]
        H_POST2["🪝 post-plan"]
    end

    APPROVED --> PSUP
    PSUP --> H_LT2 --> H_PP2 --> H_DEC2
    H_DEC2 --> H_CL2

    subgraph "Atomic Execution Loop (per phase)"
        LSUP["**Loop Supervisor**\nprofile: supervisor · primary"]
        H_LT3["🪝 load-task"]
        IMPL["**Implementor**\nprofile: worker · subagent"]
        IMPL_H_LT["🪝 load-task"]
        CREV["**Code Reviewer**\nprofile: reviewer · subagent\n(fresh context)"]
        CREV_H_LT["🪝 load-task"]
        QAR["**QA Runner**\nprofile: qa · subagent"]
        QAR_H_POST["🪝 post-plan"]
        H_POST3["🪝 post-plan"]
    end

    H_CL2 --> LSUP
    LSUP --> H_LT3 --> IMPL
    IMPL --> IMPL_H_LT
    IMPL --> CREV
    CREV --> CREV_H_LT
    CREV --> QAR
    QAR --> QAR_H_POST --> LSUP
    LSUP --> H_POST3

    subgraph "Recovery (on FAIL / BLOCKED)"
        DBG["**Expert Debugger**\nprofile: readonly · subagent"]
        DBG_H_POST["🪝 post-plan"]
    end

    LSUP -.->|"failure evidence"| DBG
    DBG --> DBG_H_POST
    DBG -.->|"remediation brief"| LSUP

    H_POST3 --> H_LBL2 --> PHASE_DONE{All phases\ncomplete?}
    PHASE_DONE -- "next phase" --> H_CL2
    PHASE_DONE -- "all done" --> H_POST2 --> COMPLETE([Plan complete])

    style H_LT1 fill:#6366f1,color:#fff,stroke:none
    style H_CL fill:#6366f1,color:#fff,stroke:none
    style H_PP1 fill:#a855f7,color:#fff,stroke:none
    style H_LBL fill:#6366f1,color:#fff,stroke:none
    style H_DEC fill:#6366f1,color:#fff,stroke:none
    style H_POST1 fill:#6366f1,color:#fff,stroke:none
    style H_LT2 fill:#6366f1,color:#fff,stroke:none
    style H_CL2 fill:#6366f1,color:#fff,stroke:none
    style H_PP2 fill:#a855f7,color:#fff,stroke:none
    style H_DEC2 fill:#6366f1,color:#fff,stroke:none
    style H_LBL2 fill:#6366f1,color:#fff,stroke:none
    style H_POST2 fill:#6366f1,color:#fff,stroke:none
    style H_LT3 fill:#6366f1,color:#fff,stroke:none
    style IMPL_H_LT fill:#6366f1,color:#fff,stroke:none
    style CREV_H_LT fill:#6366f1,color:#fff,stroke:none
    style QAR_H_POST fill:#6366f1,color:#fff,stroke:none
    style H_POST3 fill:#6366f1,color:#fff,stroke:none
    style DBG_H_POST fill:#6366f1,color:#fff,stroke:none
```

## Agent Role Summary

| Agent | Profile | Mode | Isolation | Hooks |
|---|---|---|---|---|
| [Planner](planner.md) | `planner` | primary | sandbox | load-task · classify · pre-plan · label · decompose · post-plan |
| [Plan Reviewer](plan-reviewer.md) | `reviewer` | subagent | sandbox | pre-plan |
| [Plan Supervisor](plan-supervisor.md) | `supervisor` | primary | workspace | load-task · classify · pre-plan · decompose · label · post-plan |
| [Loop Supervisor](loop-supervisor.md) | `supervisor` | primary | workspace | load-task · post-plan |
| [Implementor](implementor.md) | `worker` | subagent | sandbox | load-task |
| [Code Reviewer](code-reviewer.md) | `reviewer` | subagent | sandbox | load-task |
| [QA Runner](qa-runner.md) | `qa` | subagent | sandbox | post-plan |
| [Expert Debugger](expert-debugger.md) | `readonly` | subagent | sandbox | post-plan |

## Hook Event Reference

| Event | Registered by | Typical purpose |
|---|---|---|
| `load-task` | Planner · Plan Supervisor · Loop Supervisor · Implementor · Code Reviewer | Resolve task / brief from host task-system or context |
| `pre-plan` | Planner · Plan Supervisor · Plan Reviewer | Validation gate before planning or review begins |
| `classify` | Planner · Plan Supervisor | Route to destination / select next unblocked phase |
| `label` | Planner · Plan Supervisor | Apply task-system labels or state transitions |
| `decompose` | Planner · Plan Supervisor | Project child phases into task-system |
| `post-plan` | Plan Supervisor · Loop Supervisor · QA Runner · Expert Debugger | Signal completion / publish / notify |

Hooks are resolved once at install/sync time from `~/.agent-hooks/`. Markdown instructions
take precedence over executable scripts. When no hook is installed, agents continue without it.
