# Agent Fabric Domain

Agent Fabric is a portable, provider-neutral agent definition and multi-harness adaptation system.
It establishes canonical role definitions and compiles them deterministically into native configurations
and prompt files for AI developer tools. This glossary defines the canonical terminology used across
code, configuration, and documentation.

## Language

**Canonical Agent Definition**:
The single source of truth Markdown file in `agents/<id>.md` containing schema version 1 frontmatter,
role boundaries, portable workflows, permissions, and declarative hook placeholders.
_Avoid_: generated agent, agent template, wrapper, prompt file

**Adapter**:
A target-specific specification (`adapters/<target>.json`) and Go rendering module (`internal/adapter/`)
that translates canonical definitions, profile tiers, permissions, and tool IDs into harness-native agent formats.
_Avoid_: compiler, transpiler, converter, exporter

**Harness**:
The runtime host environment (such as OpenCode, Kilo, Antigravity CLI, Codex, or Claude Code) where
generated agent definitions are deployed and executed.
_Avoid_: platform, model provider, backend, runtime engine

**Manifest**:
The atomic JSON state file (`.agent-fabric-manifest.json`) tracking generated paths, source versions,
omissions, and SHA-256 content hashes to enable safe updates without overwriting local user customizations.
_Avoid_: lockfile, statefile, registry, tracking file

**Hook Event**:
One of the six portable lifecycle extension points (`load-task`, `pre-plan`, `classify`, `label`,
`decompose`, `post-plan`) supported by Agent Fabric; canonical agents declare their own subset via
frontmatter, resolved deterministically at install/sync time from host-global `~/.agent-hooks/`.
_Avoid_: callback, plugin, middleware, interceptor

**Profile Tier**:
An abstract compute and capability classification (`planner`, `worker`, `reviewer`, `supervisor`)
that adapters map to specific harness provider model IDs and reasoning effort levels.
_Avoid_: model name, LLM tier, prompt level, SKU

**Vertical Slice**:
A self-contained, independently delegable, testable unit of implementation work in an implementation
plan that cuts across all necessary layers (contracts, implementation, verification) rather than horizontal tiers.
_Avoid_: horizontal layer, partial phase, subtask, work package

**Delegation Packet**:
The self-locating fresh-context handoff whose normative fields and fail-closed
resolution behavior are defined in `docs/specs/agent-fabric.md`.
_Avoid_: ambient context, implicit workspace, raw transcript

**Declared Root**:
A packet-named execution, artifact, or evidence base used to interpret explicitly
based relative locators. Normative resolution rules live in `docs/specs/agent-fabric.md`.
_Avoid_: current directory assumption, discovered home, implicit base

**Authoritative Locator**:
An unambiguous reference to authoritative packet input under the declared execution
context. Accepted forms and failure behavior are defined in `docs/specs/agent-fabric.md`.
_Avoid_: bare filename, search hint, guessed path

**Hub Catalog**:
A curated external collection of supplemental agent definitions (`hub.json`) that can be installed
and verified against existing core Fabric dependencies.
_Avoid_: plugin store, agent marketplace, package manager
