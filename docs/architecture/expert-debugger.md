# Expert Debugger — Workflow Diagram

`agents/expert-debugger.md` · profile: `readonly` · mode: `subagent` · isolation: `sandbox`

The Expert Debugger diagnoses hard failures and produces a bounded root-cause remediation brief.
It cannot modify files or invent task-system state. It is invoked by the loop-supervisor
during bounded recovery.

## Full Workflow

```mermaid
flowchart TD
    START([Failure evidence from loop-supervisor]) --> HYPO

    subgraph "Hypothesis Construction"
        HYPO["Construct at least 3 distinct hypotheses\nfrom exact evidence\n(logs · call paths · config · reproduction)"]
    end

    HYPO --> ELIMINATE

    subgraph "Elimination"
        ELIMINATE["Eliminate hypotheses against:\n• Logs and stack traces\n• Call / execution paths\n• Configuration state\n• Safe bounded reproduction commands"]

        CLASSIFY["Distinguish root cause type:\n• Environment trap\n• Code defect\n• Specification drift\n• Flaky integration"]

        OSCILLATION["Detect retry oscillation:\nTrace FIRST invalid assumption,\nnot downstream symptoms"]
    end

    ELIMINATE --> CLASSIFY --> OSCILLATION --> ROLLBACK_CHECK

    subgraph "Rollback Assessment"
        ROLLBACK_CHECK{"Rollback\nneeded?"}
        ROLLBACK_TARGET["Name verified checkpoint\n(do NOT reset/mutate workspace)"]
    end

    ROLLBACK_CHECK -- Yes --> ROLLBACK_TARGET --> HANDOFF_BUILD
    ROLLBACK_CHECK -- No --> HANDOFF_BUILD

    subgraph "Handoff Artifact"
        HANDOFF_BUILD["Build compact handoff artifact:\n• Diagnosis summary\n• Affected symbols\n• Constraints\n• Smallest safe remediation steps\n• Exact verification steps\n• Rollback facts\n• Unresolved risks"]
    end

    HANDOFF_BUILD --> PP

    subgraph "Post Hook"
        PP["post-plan hook\n(if installed)\nRecord diagnostic completion"]
    end

    PP --> OUTPUT

    subgraph "Output"
        OUTPUT["Return machine-readable diagnosis"]
    end

    OUTPUT --> DONE([Return bounded remediation brief to loop-supervisor])

    style PP fill:#6366f1,color:#fff,stroke:none
```

## Hook Summary

| Hook | When | Purpose |
|---|---|---|
| `post-plan` | After diagnostic handoff | Notify / record diagnostic completion |

## Constraints

| Allowed | Not Allowed |
|---|---|
| Run safe bounded reproduction commands | Modify any product files |
| Read logs, traces, configuration | Reset or clean workspace |
| Name a verified rollback checkpoint | Invent task-system state |
| Return bounded remediation brief | Apply the fix itself |
| Use `bash` to inspect only | Stash, checkout, or overwrite unknown changes |

## Output Contract

```json
{
  "failure_classification": "infinite_loop|cascading_error|silent_tool_failure|environment_drift|quality_bypass",
  "root_cause_analysis": {
    "culprit": "",
    "chronology": "",
    "blockers": []
  },
  "tactical_intervention": {
    "remediation_type": "surgical_patch|rollback|environment_reset",
    "rollback_target": null,
    "instructions": "",
    "handoff_artifact": ""
  },
  "suggested_worker_overrides": {
    "poka_yoke_rules": [],
    "required_checks": []
  }
}
```

## Investigation Principle

> Trace execution **backward** to the first invalid state or assumption — not to the most
> visible symptom. Inspect repeated attempts for oscillation, environment drift, swallowed
> failures, and test bypasses.
