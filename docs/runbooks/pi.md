# Pi

Fabric targets Pi's official optional subagent example extension, not a native
agent registry. Install Pi first (`npm install -g @earendil-works/pi-coding-agent`).
From a Fabric checkout, install the runtime without copying upstream source:

```bash
PI_PACKAGE="$(npm root -g)/@earendil-works/pi-coding-agent"
test -f "$PI_PACKAGE/examples/extensions/subagent/index.ts"
mkdir -p ~/.pi/agent/extensions/subagent
ln -s "$PI_PACKAGE/examples/extensions/subagent/index.ts" ~/.pi/agent/extensions/subagent/index.ts
ln -s "$PI_PACKAGE/examples/extensions/subagent/agents.ts" ~/.pi/agent/extensions/subagent/agents.ts
agf install --source "$PWD" --tools pi --all --yes
```

Use `${PI_CODING_AGENT_DIR}` instead of `~/.pi/agent` if customized. Preserve
existing extension files; the commands deliberately fail rather than replace them.
Restart Pi or `/reload`. Ask it to use the `subagent` tool with
`{"agent":"planner","task":"..."}`. Agent names are not built-in slash commands.
The upstream example does not provide Fabric's resumable child-session contract;
report that capability gap when a workflow requires continuation, rather than
claiming a new stateless child resumed prior work.

Model defaults inherit the dispatching session, including thinking level. Set Pi's
active model using `/model` or its normal provider configuration. Optional Fabric
profile overrides use `profiles.pi.<profile>.model` and `.effort` in the existing
Fabric user config. Fabric does not copy provider credentials or change Pi defaults.
Portable lifecycle hooks are resolved from `~/.agent-hooks/` into agent instructions
at install/sync time. They are agent-run commands, not automatic Pi event listeners.
Read-only roles require their dispatcher to supply shell-hook results. Native
retrospective observation is not installed for Pi. Builtin tool lists are not an OS
sandbox and do not remove MCP capabilities; review trusted extensions separately.

For the browser workspace: `npm install -g @pi-chamber/web`, then
`pichamber serve --host 127.0.0.1 --port 32145`. Keep it on loopback for local
evaluation; configure authentication before authorizing any LAN/public exposure.
PiChamber runs its bundled Pi SDK, which may differ from the global CLI version.
Check `pichamber status` and the browser UI separately from `pi --version`.
