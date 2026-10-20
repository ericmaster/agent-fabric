# Pi

Fabric renders portable agents for Pi's optional upstream subagent extension,
not a built-in named-agent registry. Preserve existing extensions and use the
installed Pi package's example when enabling that optional runtime:

```bash
PI_PACKAGE="$(npm root -g)/@earendil-works/pi-coding-agent"
test -f "$PI_PACKAGE/examples/extensions/subagent/index.ts"
mkdir -p ~/.pi/agent/extensions/subagent
ln -s "$PI_PACKAGE/examples/extensions/subagent/index.ts" ~/.pi/agent/extensions/subagent/index.ts
ln -s "$PI_PACKAGE/examples/extensions/subagent/agents.ts" ~/.pi/agent/extensions/subagent/agents.ts
agf install --source "$PWD" --tools pi --all --yes
```

Use `PI_CODING_AGENT_DIR` when customized. The links deliberately fail rather
than replace existing files. Reload only an owned idle session. Model defaults
inherit the dispatching session; Fabric does not copy credentials or change Pi's
model configuration. Built-in tool lists and extension code are not an OS sandbox.
Portable canonical agent hooks are explicit agent-run instructions, not native
event subscribers; existing upstream/user extensions are independent of task review.

## PiChamber

Keep the UI on loopback; authentication is required before authorizing broader
exposure. `pichamber status` and the UI describe the actual host, which may use a
different Pi version from the global `pi` command. Do not patch installed packages
or infer native child continuation from a new stateless child.

## Task review

Invoke `skills/task-review/SKILL.md` after a completed task. Use the host's existing
native session creation/history tools to start an independent visible root with
the original session ID. No Pi retrospective extension, pinned SDK runner,
GLiDE, hook-driven context or background drain is needed. Existing provider/model
selection remains the host's responsibility.

The skill's recorder maps both Pi and PiChamber to one private `pi.jsonl`; scoped
corrections run autonomously in worktrees with checks and independent patch review.
See [`task-retrospective.md`](task-retrospective.md) for audit and hook retirement.
