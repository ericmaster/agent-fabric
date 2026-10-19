# OpenCode 2 supervision

## Verify the actual host

Check the running server's authenticated `/api/info`, not just `opencode --version`.
Keep credentials in the approved loader/process environment; emit version/status only.
On this host the two APIs (4097 and OpenChamber-managed 41629) reported 2.0.30;
the CLI reported 2.0.31. That alone does not justify changing an observer allowlist.
Do not restart live sessions to test new agent definitions.

## Native contract

Fabric's OpenCode renderer targets V2: `model: provider/model#variant`, boolean
`hidden`, and ordered `permissions` with action/resource/effect. Portable bash/task
become shell/subagent. Tool allowlists become permission rules; role denies survive.
Network-deny covers webfetch/websearch, not OS egress or arbitrary MCP services.
This is not an OS sandbox. Other harnesses retain their own rendering.

Portable `~/.agent-hooks/` commands are explicit instructions resolved by `agf sync`.
They are not native OpenCode plugin hooks. Native plugins need V2 default setup,
domain hook registration and cleanup, not a returned V1 hook object. Verify the
installed plugin ID, generation/capabilities and error path before changing it.
The optional retrospective observer is separately owned/released; changing agent
definitions does not install, enable or certify that feature.

## Dispatch and continuation

On 2.0.30 the native tool is `subagent`; continuation input is `sessionID`.
Store the returned child ID under the supervisor role's ledger entry. Resume only
that parent's child. Foreground completion returns `status: completed`; background
returns `status: running` and registers an automatic completion notification.
Use background only for non-overlapping work. End the parent turn while waiting if
nothing independent remains; the registered notification resumes delivery. Do not
create monitor agents, send status prompts, poll, or duplicate a live child.
Busy/retry is in-flight; idle alone is neither PASS nor command completion.
Reconcile a matching completed dispatch once, persist the transition and advance.
Compaction is not continuation unavailability. Preserve objective, authority,
pending stage, child handles and unresolved findings in the compact checkpoint.

Nested supervisor delegation needs an adequate **existing**
`experimental.subagent_depth` setting. Inspect it once on depth failure; honor the
host's cap instead of silently increasing it or treating a model change as recovery.
Primary-only agents cannot be launched as children; Fabric supervisors use mode
`all`. Keep one plan supervisor as delivery owner; a loop returns structural
recovery to it rather than launching a second owner for the same plan.

## Rollout and checks

Run `go test ./...`, `python3 -m unittest discover -s tests -p 'test_supervisor_*.py'`
and `agf validate --source /path/to/agent-fabric`. Use normal guarded `agf sync
--source /path/to/agent-fabric --all --tools opencode --yes`; inspect preserved
modified files instead of forcing them. Verify the actual API's effective agent
model, permissions and prompt, not merely the generated file. Legacy external hub
agents need refresh from their owning source; exact model-only overrides preserve
V2 scoped rules, and tool changes require owning-source re-rendering.
Existing sessions may retain prior instructions; new definitions do not retroactively
clear task-specific caps or authorizations. No provider call is needed to inspect
rendering/registration. Runtime verification without a provider call does not prove
autonomous multi-stage delivery; retain that qualification.

OpenChamber can independently inject a mandatory dirty-checkout worktree offer into
both the tool description and session knowledge. Its installed runtime exposes only
ask/auto (the `worktreeMoveWithoutAsking` choice), not an inspect-and-stay policy.
Fabric cannot override that higher-priority instruction. Implementing that host
preference needs a supported upstream option; do not patch installed package files
or disable issue-linking/session tools as a workaround. The Fabric checkout default
applies when the host permits it.

## Authoritative references (2.0.30)

- [Agent schema](https://github.com/anomalyco/opencode/blob/v2.0.30/services/www/src/docs/content/agents.mdx)
- [Native subagent lifecycle](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/core/src/tool/plugin/subagent.ts)
- [V1→V2 plugins](https://github.com/anomalyco/opencode/blob/v2.0.30/services/www/src/docs/content/build/plugins/migrate-v1.mdx)

Current Context7 results mix V1 and V2 documentation. Use them for discovery, then
check exact release source and the installed server contract before adopting names.
