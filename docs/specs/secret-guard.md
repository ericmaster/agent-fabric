---
title: "Spec: read-path secret guard"
type: spec
status: active
covers: internal/guard
last_checked: 2026-10-28
---

# Read-path secret guard

This intermediate implementation checks read requests only. It is not shell
protection, a sandbox, or complete secret-disclosure prevention. No harness hooks
are installed. Shell analysis and aggregate Phase 1 acceptance remain deferred.

## Protocol

`agent-fabric guard check [--log-file ABSOLUTE_PATH]` accepts exactly one JSON
object on stdin, at most 65536 bytes, and emits one JSON response on stdout.
Required read fields: `version: 1`, `op: "read"`, absolute `cwd`, nonempty string
array `paths`. Empty paths and NULs are invalid. Optional `source` is empty or
`cli`, `sdk`, `pi`, `opencode`. Unknown fields, null fields, wrong types and extra
JSON values are invalid. Read requests must not contain nonempty `command` or
`shell`. Neither submitted commands nor path contents are executed/read.

Responses contain only `version: 1`, `decision`, `reason`, `policy_version: 1`:

| Decision | Reason | Exit |
|---|---|---|
| allow | no_match | 0 |
| deny | proc_environ_read | 1 |
| error | invalid_request, limit_exceeded, unsupported_shell | 2 |

Validly formed shell requests return `error/unsupported_shell`, including empty
commands; there is no shell parser or shell policy in this tree. Errors never
echo request text, paths or values.

## Read matching

Deny `/proc/<component>/environ` and
`/proc/<component>/task/<component>/environ`, including literal wildcard
components, relative paths and symlink targets. Lexical matching is deny-only,
including nonexistent proc targets. Metadata traversal preserves raw component
order, resolving symlinks before following `..`; it uses only Lstat/Readlink,
bounded to 40 links and 8192 steps, without glob expansion or content opens.
Unmatched, missing, cyclic and over-bound paths allow; this is a narrow checker,
not proof that a later read is safe. Metadata checks and later reads are not
atomic and cannot prevent replacement races.

## Denial audit

Linux writes denial metadata to
`${XDG_STATE_HOME}/agent-fabric/guard/denials.jsonl` when that root is absolute,
otherwise `$HOME/.local/state/agent-fabric/guard/denials.jsonl`.
`--log-file` is a trusted caller-selected absolute destination, not request data.
Allow/error decisions create no audit state.

Records contain only UTC timestamp, random event_id, source (default `cli`),
rule, policy_version and layer (`deterministic`); never request text, paths,
cwd, values or hashes. Created directories are0700, files0600. Existing final
parent must be owned0700; files must be owned regular0600 with one link.
Descriptor-relative no-follow traversal rejects symlinks. A shared lock bounds
contention to approximately200ms. Rotation at1MiB retains current and one
previous file plus lock. Audit failure preserves the decision/exit and emits
only `guard_audit_unavailable` on stderr. Non-Linux audit fails explicitly
rather than approximating the Linux guarantees.

## Verification partition

The immutable70-case corpus is retained; only R01–R09 are active in policy and
compiled CLI corpus suites. Shell rows are data, not skipped/passing tests.
Read suites cover protocol, bounds, privacy, metadata-only FIFO handling,
SG-R3 symlink/`..`, denial-audit permissions, rotation, concurrency and failures.
