# QA Runner — Workflow Diagram

`agents/qa-runner.md` · profile: `qa` · mode: `subagent` · isolation: `sandbox`

The QA Runner performs deterministic and visual verification against the original DoD.
It cannot edit product files. It is dispatched by the loop-supervisor and returns a
structured evidence report.

## Full Workflow

```mermaid
flowchart TD
    START([DoD + required commands from loop-supervisor]) --> READDOD

    subgraph "Preparation"
        READDOD["Read original DoD before any testing\n(never substitute inferred DoD)"]
    end

    READDOD --> REGRESSION

    subgraph "Verification Sequence"
        REGRESSION["① Run exact required regression\nand feature checks"]
        TYPE["② Type checks\n(when applicable)"]
        RUNTIME["③ Runtime checks\n(when destination provides safe capabilities)"]
        PERSIST["④ Persistence / payload checks\n(when applicable)"]
        VISUAL["⑤ Visual / browser checks\n(when destination provides safe capabilities)"]
    end

    REGRESSION --> TYPE --> RUNTIME --> PERSIST --> VISUAL --> ANTICHECK

    subgraph "Anti-Cheat Checks"
        ANTICHECK["Detect hollow mocks\nand test bypasses"]
        MUTATION["When feasible: prove new test\nwould FAIL without implemented behavior"]
        CIRCUIT["Apply circuit breaker\nto command/browser loops\n(preserve state → return BLOCKED)"]
    end

    ANTICHECK --> MUTATION --> CIRCUIT --> PP

    subgraph "Post Hook"
        PP["post-plan hook\n(if installed)\nNotify / record QA completion"]
    end

    PP --> REPORT

    subgraph "Output"
        REPORT["Return structured evidence report\nCompress logs → relevant errors + evidence\n(no raw output forwarding)"]
    end

    REPORT --> DONE([Return to loop-supervisor])

    style PP fill:#6366f1,color:#fff,stroke:none
```

## Hook Summary

| Hook | When | Purpose |
|---|---|---|
| `post-plan` | After QA report produced | Notify / signal QA completion to task-system or downstream |

## Constraints

| Allowed | Not Allowed |
|---|---|
| Run exact required commands | Edit product files |
| Run additional safe verification | Deploy |
| Capture screenshots / browser evidence | Hide or suppress failures |
| Compress logs into relevant evidence | Replace a required command with a narrower substitute |
| Return `BLOCKED` for budget-exhausted loops | Claim `PASS` without concrete per-item evidence |

## Output Contract

```json
{
  "outcome_verdict": "PASS|FAIL|BLOCKED",
  "contract_compliance": {
    "dod_verified": false,
    "satisfied_criteria": [],
    "unmet_criteria": []
  },
  "automated_tests": {
    "total_run": 0,
    "passed": 0,
    "failed": 0,
    "mutation_check": "PASS|FAIL|NOT_AVAILABLE",
    "raw_errors_summary": ""
  },
  "visual_e2e_testing": {
    "status": "PASS|FAIL|NOT_AVAILABLE",
    "screenshots_captured": [],
    "ui_bugs": []
  },
  "anti_cheat_logs": {
    "test_bypass_detected": false,
    "hollow_mocks_detected": false
  },
  "detailed_bug_report": {
    "summary": "",
    "execution_trace_log_path": ""
  }
}
```

A status code alone is never sufficient evidence — observable behaviour and relevant side
effects must be verified.
