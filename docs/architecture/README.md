# Agent Workflow Diagrams

Architecture diagrams for each built-in Agent Fabric agent. Each diagram shows the agent's
internal workflow, hook invocations, delegation relationships, and output contracts.

| Agent | Role | Profile | Hooks |
|---|---|---|---|
| [planner](planner.md) | Authors implementation plans | planner | load-task · classify · pre-plan · label · decompose · post-plan |
| [plan-supervisor](plan-supervisor.md) | Orchestrates multi-phase plan execution | supervisor | load-task · classify · pre-plan · decompose · label · post-plan |
| [loop-supervisor](loop-supervisor.md) | Drives one atomic task to DoD | supervisor | load-task · post-plan |
| [implementor](implementor.md) | Makes bounded code changes | worker | load-task |
| [code-reviewer](code-reviewer.md) | Adversarial read-only gate | reviewer | load-task |
| [qa-runner](qa-runner.md) | Deterministic verification | qa | post-plan |
| [expert-debugger](expert-debugger.md) | Root-cause diagnosis | readonly | post-plan |
| [plan-reviewer](plan-reviewer.md) | Independent plan validation | reviewer | pre-plan |

Filing rule: these are structural diagrams — they belong in `docs/architecture/` per §3 of
[`docs-organization-blueprint.md`](../docs-organization-blueprint.md).
