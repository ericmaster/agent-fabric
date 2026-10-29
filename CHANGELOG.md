# Changelog

Preserve published release entries here, grouped by actual version and release date.
Earlier unrecorded history remains in Git; do not invent releases or dates.

## [Unreleased]

### Added

- Standalone shell AST inspection with pinned parser, full frozen shell/read corpus and supported wrapper-state regressions; no harness installation or enforcement claim.
- Pi secret-guard source checks stock bash and read tool calls before execution, using the native-resolved read target. Custom spawn, replaced tools, `pi.exec`, and `!` remain unsupported. No installer or host activation.
- Intermediate JSON `guard check` read-path checker with symlink-before-`..` proc matching and private Linux denial audit. Shell requests explicitly return unsupported; no harness integration.
- Instincts on reusable lessons: optional `confidence` in [0.3, 0.9] and `applicability` (`project` default | `global`) on `LessonCandidate`, with validation, read filtering, and advisory-only contracts (no auto-apply).

### Changed

- Permit explicitly user-approved harness-review correction scope amendments through a native approval locator, preserving the source/directory, append-only history and shared two-start limit.
- Make validation-to-mutation sequencing fail closed and bound implementor dispatches at structural recovery handback (instruction contracts, not runtime enforcement).
- Defer automatic harness-review loading and launch until main-task completion, including required checks, authorized delivery and cleanup; retain friction evidence during execution and allow early review only on explicit user request.
- Route harness-review by assigned role before dispatch; keep correction and patch review as leaves under one supervisor, honor skill-repair/opt-out scope, and reject recorded child identities reused as review sources before launch or audit append.
- Show root/subcommand help, including sync and hub install flags, with successful help exits; preserve unknown-flag failures.
- Require a pinned measurable evaluation baseline before phase/atomic implementation and candidate-linked criterion evidence before acceptance; preserve change history, retry budgets and discretionary QA. Add canonical role and OpenCode rendering regression guards (instruction contracts, not runtime enforcement).
- Preserve explicit OpenCode `external_directory` guard policies after tool allowlists without widening tool access or overriding role denials.
- Preserve standing maintenance authority in harness-review handoffs instead of restricting correction-owning supervisors to read-only investigation; include checkout cleanup in the activation pointer.
- Freeze correction scope/authority at the first correction start, allowing later user approval and varied earlier review prose without rewriting audit history or resetting the source's two-attempt limit.
- Rename the active review skill to `harness-review`; deprecate `task-review` as a model-disabled redirect without scripts. Add a persisted supervisor–worker investigation, supervisor evidence gates for new blocked audits, and an opt-in short native-context activation pointer; preserve historical audits and correction budgets.

### Fixed

- Reject an invalid ledger `next_stage` with the complete accepted stage vocabulary and no echo of the supplied value, without writing state; align the specification's checkpoint stage list with the hook's accepted set.
- Document verified OpenCode native history reads (`opencode api` operation discovery, `session.get`/`session.message.list`, cursor and truncation handling) with a read-only database fallback, and add targeted process-inspection plus preflight-before-mutation guidance for recovered execution detours.

## [0.2.1] - 2026-10-23

### Added

- Optional Pi package with `/agf-mode` role selection from rendered agents, exact tool-call allowlists, branch-aware persistence and prior-tool restoration; included in release archives and staged by bootstrap without automatic activation.

### Changed

- Stage a private Node executable for release CI's strict Pi runtime fixtures rather than depending on the runner binary's writable permission mode.
- Require durable task-review remediation: scoped SSOT scripts for repeated quoting failures, reusable guidance for resolved search/tool detours, and one bounded diagnostic subagent for unknown causes followed by authorized repair or explicit not-remediated evidence.
- Restore task-review's execution-learning purpose: diagnose harness/tool use, orchestration and skill/context clarity even after task success; require evidence-linked preventive actions and schema-2 execution rows while preserving legacy audits.
- Clarify discretionary task-review triggers for completion, roadblocks, repeated errors and stalls; support asynchronous visible PiChamber root reviews through the existing native API without result-return gating or new dispatch infrastructure.

- Make `CHANGELOG.md` the release-history owner and align ADR guidance with the pre-release/post-release lifecycle.
- Replace native retrospective hooks with a model-invoked `task-review` skill, visible independent session review, worktree correction and per-engine JSONL action audit.
