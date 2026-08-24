# Loop Supervisor — Workflow Diagram

`agents/loop-supervisor.md` · profile: `supervisor` · mode: `all` · isolation: `workspace`

The Loop Supervisor drives one atomic task through implementation, review, testing, and
DoD validation. It owns the control loop and evidence integrity; it never self-certifies work.

## Full Workflow

```mermaid
flowchart TD
    START([Phase brief from plan-supervisor]) --> LT

    subgraph "Load Task"
        LT["load-task hook\nLoad atomic task from supplied brief,\nlocal artifact, or task-system adapter"]
        VALIDATE{"Task has:\nobjective · bounded scope · guidance\nobservable DoD · required cmds · rollback path?"}
        BLOCKED_EARLY([Return BLOCKED\n— design gap])
    end

    LT --> VALIDATE
    VALIDATE -- No --> BLOCKED_EARLY
    VALIDATE -- Yes --> BRIEF

    subgraph "Atomic Execution Loop"
        BRIEF["① Create focused brief\n(objective · non-goals · scope · guidance\npermitted paths · DoD · gates · rollback · VCS state)"]

        IMPL["② Dispatch implementor\nin fresh context\n(bounded change only)"]

        REV["③ Dispatch code-reviewer\nin fresh context\n(brief + diff · findings are evidence)"]

        QA["④ Dispatch qa-runner\n(original DoD · exact required commands\nruntime + visual evidence)"]

        RECONCILE["⑤ Reconcile all reports\nagainst original task\nRecord final state · update host task"]
    end

    BRIEF --> IMPL --> REV --> QA --> RECONCILE

    subgraph "Outcome"
        RESULT{Verdict?}
    end

    RECONCILE --> RESULT

    subgraph "Bounded Recovery"
        PRESERVE["Preserve evidence\nClassify blocker\n(defect · env · spec-drift · flaky)"]
        DIAG_CTX["Fresh diagnostic context\n(expert-debugger if needed)"]
        REBR["Re-brief same task\n+ diagnostic artifact"]
        BUDGET{Recovery budget\nremaining?}
        ESCALATE_ART["Produce escalation artifact\nStop"]
    end

    RESULT -- "FAIL / BLOCKED" --> PRESERVE --> DIAG_CTX --> REBR --> BUDGET
    BUDGET -- Yes --> IMPL
    BUDGET -- No --> ESCALATE_ART --> REPORT

    RESULT -- PASS --> REPORT

    REPORT["Return machine-readable report\n{status · dod[] · required_gates[]\nremaining_blockers[] · changed_files[]}"]
    REPORT --> DONE([Hand off to plan-supervisor])

    style LT fill:#6366f1,color:#fff,stroke:none
    style IMPL fill:#0ea5e9,color:#fff,stroke:none
    style REV fill:#0ea5e9,color:#fff,stroke:none
    style QA fill:#0ea5e9,color:#fff,stroke:none
```

## Delegation Model

```mermaid
flowchart LR
    LS[Loop Supervisor]
    IMP[implementor\nfresh context]
    CR[code-reviewer\nfresh context]
    QAR[qa-runner\nfresh context]
    DBG[expert-debugger\nfresh context\n— recovery only]

    LS -->|"bounded brief\n+ workspace state"| IMP
    IMP -->|"changed files\n+ evidence"| LS
    LS -->|"brief + diff"| CR
    CR -->|"findings JSON"| LS
    LS -->|"DoD + commands"| QAR
    QAR -->|"QA report"| LS
    LS -.->|"failure artifacts\n(recovery path)"| DBG
    DBG -.->|"remediation brief"| LS
```

## Hook Summary

| Hook | When | Purpose |
|---|---|---|
| `load-task` | Start | Resolve and validate atomic task; block if design gap |

## Output Contract

```json
{
  "status": "PASS|FAIL|BLOCKED",
  "dod": [{"item": "original DoD", "status": "PASS|FAIL|BLOCKED", "evidence": "path or command"}],
  "required_gates": [{"command": "exact command", "status": "PASS|FAIL|BLOCKED", "evidence": "path"}],
  "remaining_blockers": [],
  "changed_files": []
}
```

`PASS` requires every original DoD item and every mandatory gate to have concrete passing evidence.
