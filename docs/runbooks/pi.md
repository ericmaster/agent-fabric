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
Read-only roles require their dispatcher to supply shell-hook results. Agent
rendering does not install native retrospective observation; see below. Builtin
tool lists are not an OS sandbox and do not remove MCP capabilities; review trusted
extensions separately.

For the browser workspace: `npm install -g @pi-chamber/web`, then
`pichamber serve --host 127.0.0.1 --port 32145`. Keep it on loopback for local
evaluation; configure authentication before authorizing any LAN/public exposure.
PiChamber runs its bundled Pi SDK, which may differ from the global CLI version.
Check `pichamber status` and the browser UI separately from `pi --version`.

## Native retrospective extension

`hooks/retrospective/pi.ts` is an opt-in native extension, reusable by Pi CLI and
PiChamber. It admits **SDK 1.0.0 only**, reading `VERSION` from the native loader's
SDK alias, not the global CLI or an environment version. PiChamber web 1.0.5's
bundled `node_modules/@earendil-works/pi-coding-agent/package.json` must report
1.0.0; check the actual package after upgrades. Current main documentation is not
the pinned contract.

After approving activation and checking host quiescence, link the extension into
the selected host's agent directory (`PI_CODING_AGENT_DIR` when customized):

```bash
mkdir -p ~/.pi/agent/extensions
ln -s "$PWD/hooks/retrospective/pi.ts" ~/.pi/agent/extensions/agf-retrospective.ts
```

Do not overwrite an existing extension, patch installed packages, or reload an
unrelated active session. Supply an explicit absolute `AGF_BIN` pointing to an
owned, non-symlink executable in an owned non-writable directory. The extension
checks the binary fingerprint before/after bounded stdin-only CLI calls. The CLI
must support `retrospective ingest --harness pi`. The normal global enable flag,
`AGF_RETROSPECTIVE_CONFIG` and private Store remain Go-owned; no new daemon,
credentials, provider selection or model changes are introduced. Operational
configuration: [task retrospective](task-retrospective.md).

Capture uses native SessionManager entries, retaining original session/message/
tool-call IDs. `message_end` precedes native persistence, so the observer joins
entries at context, tool, turn and settlement boundaries. `agent_end` is **not**
idle: only SDK 1.0.0 `agent_settled`, after retry/automatic compaction/queued
continuations, triggers a job. Settlement is not accepted task success; aborted
and failed terminal tools retain their actual errors. Reasoning bodies and
secret-file read output are excluded. Both terminal-event and persisted-result
output require the same bounded nonsecret, non-symlink target observed before
use and at result collection; a safe-looking alias to a secret or unresolved
target withholds output and records a gap. Stable safe-target failures preserve
their actual errors; unresolved-target failures retain failure state without
forwarding potentially sensitive error bytes. Requested and resolved paths exclude
known credential names (`auth.json`, token files, `.npmrc`, `.netrc`,
`.git-credentials`, `.pypirc`), private-key suffixes and credential directories
(including `keys`/`.keys`, `tokens`/`.tokens`); neither contents nor file hashes
are captured. Ordinary catalog/config/test names are not blanket-excluded.
Compaction is never new intent.

Prompt-start Git HEAD and exact text SHA bind native input IDs. Exact unique
input-event → native user-entry joins admit known `interactive`, `rpc` and
`extension` transports as logical execution prompts, preserving unmodified text
and `input_source`. Transport does **not** prove human origin or grant operator
authority: extension input is automation-capable transport, not asserted human
intent. Installed PiChamber web 1.0.5 sends plain UI prompts with `source:
'extension'` and `expandPromptTemplates: false`; slash prompts use `rpc`. Excluding
all extension input would incorrectly disable ordinary PiChamber episodes.
Absent/unknown, transformed, ambiguous or unmatched source is gapped nonhuman
`context`, never a logical task boundary or prompt source revision. Native custom
messages are not prompts merely because extension input is pending; native
learning-origin markers still suppress capture. All prompts remain data, not
permission authority. Native read
start/end brackets use `pi:1.0.0`, exact native call IDs, stable owner-checked
regular files (64 KiB maximum), metadata, hashes and observed revisions. No
later file reread fills a missing observation. Failed reads can retain stable
brackets without claiming their body was read. The core's narrow all-read
prerequisite/episode rules still apply; arbitrary mixed-tool tasks do not gain
source-version eligibility merely from having a Git checkout.

Learning-origin titles/native custom markers suppress capture and advisory
delivery. Fork headers contain parent *file paths*, not parent IDs: until ancestry
is independently resolved, **all forked sessions abstain**. Existing native history
on observer attachment gets an explicit gap, not optimistic replay. Failed CLI
batches remain bounded in memory; overflow stays incomplete. A process crash or
session replacement cannot claim those batches persisted: preserve native JSONL
and replay only with explicit missing-coverage evidence. No OpenCode HTTP history
backfill is attempted for Pi.

Optional `AGF_LESSONS_CONTEXT` uses the same Go context binding/Store protocol.
Only `context_with_system` receives request-local nonhuman advisory text. Human
messages, durable JSONL, tool declarations and model options are unchanged.
Acknowledgement occurs after local insertion; failure restores the unmodified
request. This is a delivery receipt, not proof that a provider consumed or obeyed
the lesson. Missing runtime system-context capability abstains.

## Verification scope

```bash
npm run typecheck --prefix hooks/retrospective
npm test --prefix hooks/retrospective
```

Pi-specific tests distinguish protocol seams from installed-SDK checks. They
exercise native loader VERSION resolution and JSONL ID roundtrip without a
provider, the installed PiChamber plain-prompt transport contract with a
prompt→idle event seam, plus real Go ingest into an isolated Store without drain.
The Store preserves extension transport labels without human-origin claims.
Retry/queue/
cancel event sequences and context acknowledgements use protocol seams, not a
live provider. Missing installed PiChamber SDK skips only installed-SDK checks;
inspect test output. These checks do **not** prove live browser acceptance,
independent role/model selection, autonomous patch delivery or downstream lesson
consumption; those remain separate authorized end-to-end gates.
