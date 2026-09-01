# Agent Workflow Diagrams

Architecture diagrams for each built-in Agent Fabric agent. Each diagram shows the agent's
internal workflow, hook invocations, delegation relationships, and output contracts.

| Agent | Role | Profile | Hooks |
|---|---|---|---|
| [planner](planner.md) | Authors implementation plans | planner | load-task · pre-plan · post-plan |
| [plan-supervisor](plan-supervisor.md) | Orchestrates multi-phase plan execution | supervisor | load-task · pre-plan · label · decompose |
| [loop-supervisor](loop-supervisor.md) | Drives one atomic task to DoD | supervisor | load-task |
| [implementor](implementor.md) | Makes bounded code changes | worker | load-task |
| [code-reviewer](code-reviewer.md) | Adversarial read-only gate | reviewer | load-task |
| [qa-runner](qa-runner.md) | Deterministic verification | qa | — |
| [expert-debugger](expert-debugger.md) | Root-cause diagnosis | readonly | — |
| [plan-reviewer](plan-reviewer.md) | Independent plan validation | reviewer | pre-plan |
| [deploy-supervisor](deploy-supervisor.md) | Gated release & deploy verification | supervisor | load-task · pre-deploy · post-deploy |

Filing rule: these are structural diagrams — they belong in `docs/architecture/` per §3 of
[`docs-organization-blueprint.md`](../docs-organization-blueprint.md).
