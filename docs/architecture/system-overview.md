# Agent Fabric — System Architecture

This diagram shows how the built-in agents collaborate in the full intake → planning → execution
lifecycle, including hook event fire points and delegation relationships.
Every fresh-context delegation arrow carries the self-locating packet defined
normatively in [`docs/specs/agent-fabric.md`](../specs/agent-fabric.md); hooks may
enrich or validate that packet but never reconstruct known locators.

## Full System Diagram

```mermaid
flowchart TD
    USER([User / Operator]) --> PLANNER
    USER --> BF

    subgraph "Bug Intake"
        BF["**Bug Fixer**\nprofile: supervisor · primary"]
        H_PT["🪝 persist-ticket"]
        H_LBL_BF["🪝 label"]
        RR["**Report Reviewer**\nprofile: reviewer · subagent\n(fresh context)"]
        H_DEC_BF["🪝 decompose"]
    end

    BF -->|"validated review packet"| RR
    RR -->|"review result"| BF
    BF --> H_PT --> H_LBL_BF
    BF -->|"validated planner packet"| PLANNER
    H_LBL_BF -->|"exec-ready packet"| LSUP
    BF -->|"user confirms projection"| H_DEC_BF
    H_DEC_BF -->|"validated plan-supervisor packet"| PSUP

    subgraph "Planning Pipeline"
        PLANNER["**Planner**\nprofile: planner · primary"]
        H_LT1["🪝 load-task"]
        H_PP1["🪝 pre-plan"]
        PREV["**Plan Reviewer**\nprofile: reviewer · subagent\n(fresh context)"]
        H_POST1["🪝 post-plan"]
    end

    PLANNER --> H_LT1 --> H_PP1 -->|"validated review packet"| PREV
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

    H_DEC2 -->|"validated phase packet"| LSUP
    LSUP --> H_LT3
    IMPL_H_LT -.->|"load-task"| IMPL
    CREV_H_LT -.->|"load-task"| CREV
    LSUP -->|"validated implementation packet dispatch"| IMPL
    IMPL -->|"implementation result"| LSUP
    LSUP -->|"validated review packet dispatch"| CREV
    CREV -->|"review result"| LSUP
    LSUP -->|"validated QA packet dispatch"| QAR
    QAR -->|"QA result"| LSUP

    subgraph "Recovery (on FAIL / BLOCKED)"
        DBG["**Expert Debugger**\nprofile: solver · subagent"]
    end

    LSUP -.->|"validated recovery packet"| DBG
    DBG -.->|"remediation brief"| LSUP

    LSUP --> H_LBL2 --> PHASE_DONE{All phases\ncomplete?}
    PHASE_DONE -- "next phase" --> H_DEC2
    PHASE_DONE -- "all done" --> COMPLETE([Plan complete])

    style H_PT fill:#6366f1,color:#fff,stroke:none
    style H_LBL_BF fill:#6366f1,color:#fff,stroke:none
    style H_DEC_BF fill:#6366f1,color:#fff,stroke:none
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
| [Plan Supervisor](plan-supervisor.md) | `supervisor` | primary | workspace | load-task · pre-plan · label · decompose · record-ledger |
| [Loop Supervisor](loop-supervisor.md) | `supervisor` | all | workspace | load-task · record-ledger |
| [Implementor](implementor.md) | `worker` | subagent | sandbox | load-task |
| [Code Reviewer](code-reviewer.md) | `reviewer` | subagent | sandbox | load-task |
| [QA Runner](qa-runner.md) | `qa` | subagent | sandbox | — |
| [Expert Debugger](expert-debugger.md) | `solver` | subagent | sandbox | — |
| [Bug Fixer](bug-fixer.md) | `supervisor` | primary | workspace | load-task · label · persist-ticket · decompose |
| [Report Reviewer](bug-fixer.md) | `reviewer` | subagent | sandbox | — |

## Hook Event Reference

| Event | Registered by | Typical purpose |
|---|---|---|
| `load-task` | Planner · Plan Supervisor · Loop Supervisor · Implementor · Code Reviewer | Enrich or validate supplied task packet context |
| `pre-plan` | Planner · Plan Supervisor · Plan Reviewer | Validation gate and schema/constraint loading before planning or review begins |
| `classify` | — (reserved; no built-in agent registers it) | Route to destination / select next unblocked phase |
| `label` | Plan Supervisor · Bug Fixer | Apply task-system labels or state transitions |
| `persist-ticket` | Bug Fixer | Persist a portable bug ticket; file default when uninstalled |
| `decompose` | Plan Supervisor · Bug Fixer | Project child phases into task-system |
| `post-plan` | Planner | Signal completion / publish / notify |
| `record-ledger` | Plan Supervisor · Loop Supervisor | Record structured macro- or micro-ledger event to host memory or fallback JSONL |

Hooks are resolved once at install/sync time from `~/.agent-hooks/`. Markdown instructions
take precedence over executable scripts. When no hook is installed, agents continue without it.
