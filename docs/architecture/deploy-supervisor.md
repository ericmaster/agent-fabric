# Deploy Supervisor — Workflow Diagram

`agents/deploy-supervisor.md` · profile: `supervisor` · mode: `primary` · isolation: `workspace`

The Deploy Supervisor coordinates post-merge deployment execution, database migration checks,
live endpoint smoke testing, and empirical release evidence collection. It executes deployments
only under explicit human operator confirmation.

## Full Workflow

```mermaid
flowchart TD
    START([User Release Instruction]) --> LT

    subgraph "Load Task & Profile"
        LT["load-task hook\nLoad release task, destination target, and\nproject profile (.agent-fabric/profile.yaml)"]
        CONFIRM{"Explicit Operator\nAuthorization Signed?"}
        BLOCKED_GATE([Return BLOCKED\n— unauthorized deploy attempt])
    end

    LT --> CONFIRM
    CONFIRM -- No --> BLOCKED_GATE
    CONFIRM -- Yes --> PRE_DEPLOY

    subgraph "Release Execution Sequence"
        PRE_DEPLOY["① pre-deploy hook\nValidate environment health & staging state"]
        DEPLOY["② Execute Build & Deploy\n(e.g., Pages/Workers, container, hosting cmd)"]
        MIG["③ Validate Database Migrations\n(schema status on target store)"]
        SMOKE["④ Run Route Smoke Tests\n(HTTP status, latency, payload assertions)"]
        PARITY["⑤ Assert Git Parity\n(HEAD SHA == deployed runtime tag)"]
        POST_DEPLOY["⑥ post-deploy hook\nGenerate RELEASE_EVIDENCE.md"]
    end

    PRE_DEPLOY --> DEPLOY --> MIG --> SMOKE --> PARITY --> POST_DEPLOY

    subgraph "Outcome"
        RESULT{All checks PASS?}
        REP_PASS["Return DEPLOYED / VERIFIED\nwith release artifact"]
        REP_FAIL["Return FAILED / ROLLED_BACK\nwith error trace"]
    end

    POST_DEPLOY --> RESULT
    RESULT -- Yes --> REP_PASS --> DONE([Release Complete])
    RESULT -- No --> REP_FAIL --> ESCALATE([Operator Escalation])

    style LT fill:#6366f1,color:#fff,stroke:none
    style DEPLOY fill:#d97706,color:#fff,stroke:none
    style SMOKE fill:#0ea5e9,color:#fff,stroke:none
    style POST_DEPLOY fill:#10b981,color:#fff,stroke:none
```

## Hook Summary

| Hook | When | Purpose |
|---|---|---|
| `load-task` | Start | Resolve release task, project profile, and smoke endpoints |
| `pre-deploy` | Before deploy execution | Validate target environment health and pre-deploy prerequisites |
| `post-deploy` | After verification | Generate `RELEASE_EVIDENCE.md` and notify release completion |

## Constraints

| Allowed | Not Allowed |
|---|---|
| Execute project profile build & deploy scripts | Trigger production mutations without human authorization |
| Run live HTTP smoke tests and latency assertions | Fabricate or mock live deployment evidence |
| Verify remote database migration status | Overwrite unverified releases on failure |
| Produce structured `RELEASE_EVIDENCE.md` | Suppress deployment or migration failures |

## Output Contract

```json
{
  "release_status": "DEPLOYED|VERIFIED|FAILED|ROLLED_BACK",
  "target_environment": "staging|production",
  "git_ref": "HEAD-SHA",
  "deployment_execution": {
    "command": "",
    "status": "PASS|FAIL",
    "deploy_logs_summary": ""
  },
  "smoke_tests": [
    {
      "route": "",
      "status_code": 200,
      "passed": true,
      "latency_ms": 0
    }
  ],
  "migrations": {
    "applied_count": 0,
    "status": "PASS|FAIL|NOT_APPLICABLE"
  },
  "release_evidence_artifact": "RELEASE_EVIDENCE.md"
}
```
