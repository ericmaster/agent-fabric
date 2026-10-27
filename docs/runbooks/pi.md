# Pi

Fabric renders agent definitions for two optional runtimes: the Fabric role-mode
extension for the current session, and Pi's upstream `subagent` extension for
fresh-context delegation. Pi has no built-in named-agent registry.

## Current-session roles

From this checkout:

```bash
agf install --source "$PWD" --tools pi --all --yes
pi install ./extensions/pi
```

Release archives also include `extensions/pi`; bootstrap stages it beside the
binary without enabling it. With the default bootstrap location, opt in using
`pi install "$HOME/.local/bin/extensions/pi"`.

Reload an owned idle session, then use `/agf-mode` for the selector,
`/agf-mode planner` (or any installed agent ID), and `/agf-mode off` to
restore the tools active before mode entry. For one invocation without installing:

```bash
pi -e ./extensions/pi/index.ts --agf-mode planner
```

Modes load global rendered definitions only, honoring `PI_CODING_AGENT_DIR`.
Project agent files are not implicitly trusted or loaded. Role instructions are
appended to the existing system prompt; history is retained, so switching roles
is not independent review or a fresh-context handoff. The current model/thinking
remain unchanged; rendered `model` overrides apply to subagents, not modes.
Selection follows the active session branch across resume and tree navigation.
Missing or invalid restored definitions block all tools until a valid role or
`off` is selected. Mode changes require an idle session.

Each role's exact tool allowlist governs direct and nested tool calls, including
extension/MCP tools. Tools absent from the host are omitted, not emulated.
Do not combine this with another extension managing active tools. A denied edit
role has no bash; no shell command regex claims read-only safety. This remains
an in-process workflow guard, not an OS sandbox or filesystem boundary.

Local tests reuse the installed Pi parser/loader without provider calls:
`cd extensions/pi && npm test`. Set `PI_PACKAGE` to the actual Pi package root
when testing a PiChamber-bundled host rather than the global installation.

## Fresh-context subagents

Preserve existing extensions and use the installed Pi package's example when
enabling the delegation runtime:

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

## Harness review

Invoke `skills/harness-review/SKILL.md` after main-task completion to evaluate execution friction, not at an unfinished checkpoint (unless the user explicitly requests early review). Use the host's existing
native session creation/history tools to start an independent visible root with
the original session ID. No Pi retrospective extension, pinned SDK runner,
GLiDE, hook-driven context or background drain is needed. Existing provider/model
selection remains the host's responsibility.

The skill's recorder maps both Pi and PiChamber to one private `pi.jsonl`; scoped
corrections run autonomously in worktrees with checks and independent patch review.
See [`task-retrospective.md`](task-retrospective.md) for audit and hook retirement.
